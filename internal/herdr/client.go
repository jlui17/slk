// Package herdr talks to herdr's socket API: one fresh connection per
// request — the server handles a single newline-delimited JSON request
// per connection and silently discards pipelined extras — with one JSON
// response line each. The agent-sidebar reporting (Report/ReportUnread/
// NameTab) reads responses best-effort; OpenTab parses results and
// surfaces server errors. WatchFocus is the one long-lived connection:
// an events.subscribe stream (see focuswatch.go).
package herdr

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

const (
	source      = "slk"
	sendTimeout = 500 * time.Millisecond
)

// seqCounter backs nextSeq. Process-wide is fine: one slk process reports
// for one pane.
var seqCounter atomic.Int64

// nextSeq returns a strictly increasing seq, seeded from the wall clock so
// it also outranks seqs from a previous slk run in the same pane. Bare
// UnixNano would regress on a backward clock step (NTP), and herdr silently
// drops any equal-or-stale seq — freezing the sidebar and eating the final
// release.
func nextSeq() int64 {
	for {
		prev := seqCounter.Load()
		next := time.Now().UnixNano()
		if next <= prev {
			next = prev + 1
		}
		if seqCounter.CompareAndSwap(prev, next) {
			return next
		}
	}
}

// Client is slk's client of herdr's socket API, for the herdr pane slk
// runs in. The agent-sidebar methods (Report, ReportUnread, NameTab,
// ForceNameTab) return immediately: sends run on their own goroutine,
// ordered on the herdr side by a seq captured at call time. They, the
// Set*Cache methods, WatchFocus and Close are safe on a nil receiver, so
// callers outside a herdr pane need no guards. OpenTab, OpenPane and the
// pane calls slk herdr makes block on their round-trips.
type Client struct {
	network string
	addr    string

	wg sync.WaitGroup

	// stop ends the focus watcher's reconnect loop; closed once by Close.
	stop     chan struct{}
	stopOnce sync.Once
	// retryDelay paces the focus watcher's reconnects.
	retryDelay time.Duration
	// pollInterval paces the focus watcher's location polls.
	pollInterval time.Duration

	// loadPaneID and savePaneID bridge the pane's resolved public id to
	// a store that outlives the process; both are set once via
	// SetPaneIDCache before WatchFocus and used only on the watcher's
	// goroutine (fetchPane, adopt).
	loadPaneID func() (string, bool)
	savePaneID func(paneID string) error
	// loadTabLabel and saveTabLabel bridge the last label NameTab set to
	// a store that outlives the process AND the herdr server: herdr
	// persists tab labels across its restarts but wipes pane-metadata
	// tokens, so without this record a restored label (a model-generated
	// one especially, which no rerun reproduces) reads as user-typed and
	// locks the tab out of renames. Set once via SetTabLabelCache before
	// the first NameTab; used only under tabMu.
	loadTabLabel func() (string, bool)
	saveTabLabel func(label string) error
	// savedPaneID is the last id savePaneID accepted, gating rewrites;
	// watcher goroutine only.
	savedPaneID string

	mu sync.Mutex
	// paneID, tabID, and workspaceID are the pane's current coordinates.
	// They start as the launch env's values and go stale when herdr moves
	// the pane (a cross-workspace move assigns a new public pane id); the
	// focus watcher is their only writer, adopting what the server
	// reports (see adopt). Every request reads them at build time.
	paneID      string
	tabID       string
	workspaceID string
	// terminalID is the pane's stable handle within one server run
	// (term_…): it survives moves, but a server restart or handoff
	// re-mints every terminal id (herdr persists none). Not in the
	// launch env: empty until the watcher's first successful pane.get.
	terminalID string
	// agent is the id from the last report, carried into release because
	// pane.release_agent requires it. Empty means no entry to release.
	agent string
	// watchConn is the focus watcher's live subscription connection,
	// closed by Close to unblock its read.
	watchConn net.Conn

	// tabMu serializes NameTab's read-then-rename sequence, and is separate
	// from mu so a Report on the UI goroutine never waits behind NameTab's
	// socket round-trips.
	tabMu sync.Mutex
}

// NewClientFromEnv returns a Client when slk is running inside a herdr
// pane, else nil. Requires HERDR_ENV=1 and HERDR_PANE_ID. The address comes
// from SLK_HERDR_ADDR ("host:port", TCP — used when slk runs in a container
// and a host-side bridge forwards to the socket) or else HERDR_SOCKET_PATH
// (unix socket path).
func NewClientFromEnv() *Client {
	if os.Getenv("HERDR_ENV") != "1" {
		return nil
	}
	paneID := os.Getenv("HERDR_PANE_ID")
	if paneID == "" {
		return nil
	}
	// HERDR_TAB_ID is optional: without it NameTab is a no-op and the
	// agent-sidebar reporting still works.
	tabID := os.Getenv("HERDR_TAB_ID")
	var r *Client
	if addr := os.Getenv("SLK_HERDR_ADDR"); addr != "" {
		r = newClient("tcp", addr, paneID, tabID)
	} else if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
		r = newClient("unix", path, paneID, tabID)
	} else {
		return nil
	}
	// HERDR_WORKSPACE_ID is optional: without it OpenTab is unavailable
	// (CanOpenTab false) and the sidebar reporting still works.
	r.workspaceID = os.Getenv("HERDR_WORKSPACE_ID")
	return r
}

func newClient(network, addr, paneID, tabID string) *Client {
	return &Client{
		network: network, addr: addr, paneID: paneID, tabID: tabID,
		stop:         make(chan struct{}),
		retryDelay:   watchRetryDelay,
		pollInterval: watchPollInterval,
	}
}

// Pane is the slice of a server-reported pane record that identifies
// the pane and places it.
type Pane struct {
	PaneID      string `json:"pane_id"`
	TabID       string `json:"tab_id"`
	WorkspaceID string `json:"workspace_id"`
	TerminalID  string `json:"terminal_id"`
}

func (r *Client) identity() Pane {
	r.mu.Lock()
	defer r.mu.Unlock()
	return Pane{
		PaneID:      r.paneID,
		TabID:       r.tabID,
		WorkspaceID: r.workspaceID,
		TerminalID:  r.terminalID,
	}
}

// adopt records a server-reported pane record as the pane's current
// coordinates, field-wise so a record with gaps (an older herdr omitting
// terminal_id) can't blank what is already known. Called only from the
// focus watcher's goroutine.
func (r *Client) adopt(p Pane) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p.PaneID != "" {
		r.paneID = p.PaneID
	}
	if p.TabID != "" {
		r.tabID = p.TabID
	}
	if p.WorkspaceID != "" {
		r.workspaceID = p.WorkspaceID
	}
	if p.TerminalID != "" {
		r.terminalID = p.TerminalID
	}
}

// persistPaneID hands a public id to the pane-id cache, skipping ids
// already stored and re-arming after a failed write so the next resolve
// retries it. Called only from the focus watcher's goroutine, and only
// with ids a pane.get response vouched for (locate) — never straight
// from a pane_moved event, so one malformed event can't overwrite the
// last good row.
func (r *Client) persistPaneID(paneID string) {
	if r.savePaneID == nil || paneID == "" || paneID == r.savedPaneID {
		return
	}
	if err := r.savePaneID(paneID); err != nil {
		debuglog.Notify("herdr: pane id cache write: %v", err)
		return
	}
	r.savedPaneID = paneID
}

// SetPaneIDCache connects the client to a store of the pane's
// last-resolved public id that outlives the process: save is called
// whenever a resolve lands a new id (a returned error is logged and the
// id retried on the next resolve), and load is the cold-start fallback
// for a launch-env pane id the server no longer resolves (the pane
// moved, then a restart or handoff wiped the alias map — herdr persists
// public pane ids and nothing else a pane could be found by). Must be
// called before WatchFocus; nil-safe.
func (r *Client) SetPaneIDCache(load func() (string, bool), save func(paneID string) error) {
	if r == nil {
		return
	}
	r.loadPaneID = load
	r.savePaneID = save
}

// SetTabLabelCache connects the client to a store of the last tab label
// NameTab set for this pane: save is called on every label NameTab sets or
// claims, and load backs the re-claim of a label whose herdr-side
// ownership token a herdr restart wiped (herdr persists tab labels, not
// pane metadata). Must be called before the first NameTab; nil-safe.
func (r *Client) SetTabLabelCache(load func() (string, bool), save func(label string) error) {
	if r == nil {
		return
	}
	r.loadTabLabel = load
	r.saveTabLabel = save
}

type request struct {
	ID     string `json:"id"`
	Method string `json:"method"`
	Params any    `json:"params"`
}

type reportAgentParams struct {
	PaneID  string `json:"pane_id"`
	Source  string `json:"source"`
	Agent   string `json:"agent"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
	Seq     int64  `json:"seq"`
}

type reportMetadataParams struct {
	PaneID       string `json:"pane_id"`
	Source       string `json:"source"`
	DisplayAgent string `json:"display_agent"`
	Title        string `json:"title"`
	Seq          int64  `json:"seq"`
}

type releaseAgentParams struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source"`
	Agent  string `json:"agent"`
	Seq    int64  `json:"seq"`
}

// Report upserts this pane's agent-sidebar entry: agent is herdr's internal
// agent id (e.g. "slack-claude"), displayName the human name shown in the
// sidebar (e.g. "@Claude"), title the pane title (channel + thread snippet),
// state one of herdr's lifecycle states ("working", "idle", "blocked"),
// statusMessage the transient status text ("is thinking…", may be empty).
func (r *Client) Report(agent, displayName, title, state, statusMessage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agent = agent
	paneID := r.paneID
	r.mu.Unlock()
	seq := nextSeq()
	// The metadata draws its own seq: herdr's per-pane seq counter is
	// shared across report methods and silently drops an equal-or-stale
	// seq (returning ok), which would eat the metadata every time.
	metaSeq := nextSeq()
	r.send(
		request{
			ID:     fmt.Sprintf("slk:%d", seq),
			Method: "pane.report_agent",
			Params: reportAgentParams{
				PaneID:  paneID,
				Source:  source,
				Agent:   agent,
				State:   state,
				Message: statusMessage,
				Seq:     seq,
			},
		},
		request{
			ID:     fmt.Sprintf("slk:%d", metaSeq),
			Method: "pane.report_metadata",
			Params: reportMetadataParams{
				PaneID:       paneID,
				Source:       source,
				DisplayAgent: displayName,
				Title:        title,
				Seq:          metaSeq,
			},
		},
	)
}

// ReportUnread upserts the entry through a synthetic completion: a working
// report immediately followed by idle carrying statusMessage. herdr's
// unseen "done" indicator (the sidebar's blue dot) isn't settable over its
// API; it flips only on a working→idle edge, and only when the pane's tab
// isn't both the focused workspace's focused tab and fronted by a focused
// terminal — so unread state is deliverable only as this pair, and only
// while the user isn't already looking at the pane. Both requests ride one
// send call so the idle can't overtake the working on the wire and get
// dropped as a stale seq.
func (r *Client) ReportUnread(agent, displayName, title, statusMessage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agent = agent
	paneID := r.paneID
	r.mu.Unlock()
	workingSeq := nextSeq()
	idleSeq := nextSeq()
	metaSeq := nextSeq()
	r.send(
		request{
			ID:     fmt.Sprintf("slk:%d", workingSeq),
			Method: "pane.report_agent",
			Params: reportAgentParams{
				PaneID: paneID,
				Source: source,
				Agent:  agent,
				State:  "working",
				Seq:    workingSeq,
			},
		},
		request{
			ID:     fmt.Sprintf("slk:%d", idleSeq),
			Method: "pane.report_agent",
			Params: reportAgentParams{
				PaneID:  paneID,
				Source:  source,
				Agent:   agent,
				State:   "idle",
				Message: statusMessage,
				Seq:     idleSeq,
			},
		},
		request{
			ID:     fmt.Sprintf("slk:%d", metaSeq),
			Method: "pane.report_metadata",
			Params: reportMetadataParams{
				PaneID:       paneID,
				Source:       source,
				DisplayAgent: displayName,
				Title:        title,
				Seq:          metaSeq,
			},
		},
	)
}

// release removes this pane's agent-sidebar entry. A no-op until the first
// Report, since no entry exists to remove.
func (r *Client) release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	agent := r.agent
	paneID := r.paneID
	r.mu.Unlock()
	if agent == "" {
		return
	}
	// The seq is nullable in herdr's schema but not optional in effect: a
	// missing seq counts as 0, stale against any prior report's, and herdr
	// silently ignores the release (returning ok).
	seq := nextSeq()
	r.send(request{
		ID:     fmt.Sprintf("slk:%d", seq),
		Method: "pane.release_agent",
		Params: releaseAgentParams{PaneID: paneID, Source: source, Agent: agent, Seq: seq},
	})
}

type tabGetParams struct {
	TabID string `json:"tab_id"`
}

type tabRenameParams struct {
	TabID string `json:"tab_id"`
	Label string `json:"label"`
}

type paneGetParams struct {
	PaneID string `json:"pane_id"`
}

type workspaceGetParams struct {
	WorkspaceID string `json:"workspace_id"`
}

// reportTokensParams writes only metadata tokens; display_agent and title
// are omitted entirely so an ownership write can't clear them.
type reportTokensParams struct {
	PaneID string            `json:"pane_id"`
	Source string            `json:"source"`
	Tokens map[string]string `json:"tokens"`
	Seq    int64             `json:"seq"`
}

// tabLabelToken is the pane-metadata token recording the tab label NameTab
// set. It lives in the herdr server, so ownership survives slk restarts in
// the same pane.
const tabLabelToken = "slk_tab_label"

// NameTab renames the pane's current herdr tab to label — but only over a
// label herdr assigned by default (the tab's position digits) or one
// NameTab set itself, tracked via a pane-metadata token; a label the user
// typed is never overwritten. No-op without a tab id (HERDR_TAB_ID absent
// and the focus watcher hasn't resolved one yet) and nil-safe.
func (r *Client) NameTab(label string) { r.nameTab(label, false) }

// ForceNameTab is NameTab without the ownership guard, for a rename the
// user asked for by name (:retitle): it lands over whatever label the tab
// carries and claims the tab like any label NameTab set.
func (r *Client) ForceNameTab(label string) { r.nameTab(label, true) }

func (r *Client) nameTab(label string, force bool) {
	if r == nil || label == "" {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.tabMu.Lock()
		defer r.tabMu.Unlock()
		// One identity snapshot for the whole read-then-rename sequence,
		// so the reads and the rename target one tab. A move landing
		// mid-sequence can still rename the tab the pane just left;
		// accepted, the window is a few sequential socket round-trips.
		id := r.identity()
		if id.TabID == "" {
			return
		}
		current, err := r.tabLabel(id.TabID)
		if err != nil {
			debuglog.Notify("herdr: tab.get: %v", err)
			return
		}
		if current == label {
			// Already named, possibly by a previous slk run in this pane;
			// claim it so later renames keep working.
			r.recordTabLabel(id.PaneID, label)
			r.saveTabLabelCache(label)
			return
		}
		if !force {
			owned, err := r.ownsTabLabel(id.PaneID, current)
			if err != nil {
				debuglog.Notify("herdr: pane.get: %v", err)
				return
			}
			if !owned && !isDefaultTabLabel(current) && !r.cachedTabLabel(current) {
				return
			}
		}
		if err := r.call("tab.rename", tabRenameParams{TabID: id.TabID, Label: label}, sendTimeout, nil); err != nil {
			debuglog.Notify("herdr: tab.rename: %v", err)
			return
		}
		r.recordTabLabel(id.PaneID, label)
		r.saveTabLabelCache(label)
	}()
}

// ownsTabLabel reports whether current is a label NameTab set: the pane's
// ownership token matches it.
func (r *Client) ownsTabLabel(paneID, current string) (bool, error) {
	var parsed struct {
		Pane struct {
			Tokens map[string]string `json:"tokens"`
		} `json:"pane"`
	}
	if err := r.call("pane.get", paneGetParams{PaneID: paneID}, sendTimeout, &parsed); err != nil {
		return false, err
	}
	return parsed.Pane.Tokens[tabLabelToken] == current, nil
}

// cachedTabLabel reports whether current is the label the cache says a
// previous slk run set for this pane — the recovery for a herdr restart
// having wiped the ownership token while keeping the label.
func (r *Client) cachedTabLabel(current string) bool {
	if r.loadTabLabel == nil {
		return false
	}
	last, ok := r.loadTabLabel()
	return ok && last == current
}

// saveTabLabelCache mirrors a label NameTab set into the durable cache;
// failures only cost a post-restart re-claim, so they are logged and
// dropped.
func (r *Client) saveTabLabelCache(label string) {
	if r.saveTabLabel == nil {
		return
	}
	if err := r.saveTabLabel(label); err != nil {
		debuglog.Notify("herdr: tab label cache write: %v", err)
	}
}

// recordTabLabel stores the ownership token; failures only cost a future
// rename, so they are logged and dropped. Token only, deliberately: OpenTab
// calls this for a pane it created, whose label must not enter this
// process's own label cache.
func (r *Client) recordTabLabel(paneID, label string) {
	err := r.call("pane.report_metadata", reportTokensParams{
		PaneID: paneID,
		Source: source,
		Tokens: map[string]string{tabLabelToken: label},
		Seq:    nextSeq(),
	}, sendTimeout, nil)
	if err != nil {
		debuglog.Notify("herdr: token write: %v", err)
	}
}

// tabLabel fetches the tab's current label via tab.get.
func (r *Client) tabLabel(tabID string) (string, error) {
	var parsed struct {
		Tab struct {
			Label string `json:"label"`
		} `json:"tab"`
	}
	if err := r.call("tab.get", tabGetParams{TabID: tabID}, sendTimeout, &parsed); err != nil {
		return "", err
	}
	return parsed.Tab.Label, nil
}

// isDefaultTabLabel reports whether label is a herdr-assigned default: an
// unnamed tab's label is its position rendered as digits. Capped at two
// digits so a user-typed "2024" is protected; a user label of "7" is
// indistinguishable from a default and remains the residual clobber risk.
func isDefaultTabLabel(label string) bool {
	if label == "" {
		return true
	}
	if len(label) > 2 {
		return false
	}
	for _, r := range label {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Close releases the entry, stops the focus watcher, and waits up to
// timeout for in-flight sends and the watcher to finish; called once at
// process shutdown.
func (r *Client) Close(timeout time.Duration) {
	if r == nil {
		return
	}
	r.stopOnce.Do(func() { close(r.stop) })
	r.mu.Lock()
	if r.watchConn != nil {
		r.watchConn.Close()
	}
	r.mu.Unlock()
	r.release()
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
	}
}

// send delivers each req over its own fresh connection, all on one
// goroutine so callers (the UI goroutine) never block on the socket and
// reqs from a single call stay ordered. Failures, herdr's error replies
// included, are logged and dropped: the sidebar is best-effort, and the
// next Report supersedes anything lost.
func (r *Client) send(reqs ...request) {
	lines := make([][]byte, 0, len(reqs))
	for _, req := range reqs {
		line, err := encodeRequest(req)
		if err != nil {
			debuglog.Notify("herdr: encode %s: %v", req.Method, err)
			return
		}
		lines = append(lines, line)
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for i, line := range lines {
			if _, err := r.exchange(line, sendTimeout); err != nil {
				debuglog.Notify("herdr: %s: %v", reqs[i].Method, err)
			}
		}
	}()
}

// call sends one request, waiting up to timeout for the connection and
// the reply, and decodes the reply's result object into out (nil to
// discard it); a server error response comes back as a serverError.
func (r *Client) call(method string, params any, timeout time.Duration, out any) error {
	line, err := encodeRequest(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: method,
		Params: params,
	})
	if err != nil {
		return err
	}
	result, err := r.exchange(line, timeout)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(result, out)
}

// encodeRequest renders req as one newline-terminated request line.
func encodeRequest(req request) ([]byte, error) {
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return append(line, '\n'), nil
}

// exchange sends one request line over a fresh connection and returns
// the reply's result object, or the server's error as a serverError.
func (r *Client) exchange(line []byte, timeout time.Duration) (json.RawMessage, error) {
	conn, err := net.DialTimeout(r.network, r.addr, timeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(line); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("connection closed without a reply")
	}
	return parseResponse(scanner.Bytes())
}

// serverError is an error response from herdr — the request was
// received and rejected — as opposed to a transport failure, where the
// request's server-side effect is unknown.
type serverError struct {
	code string
	msg  string
}

func (e serverError) Error() string { return e.msg }

// parseResponse returns a response line's result object, or its error
// as a serverError.
func parseResponse(line []byte) (json.RawMessage, error) {
	var parsed struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &parsed); err != nil {
		return nil, err
	}
	if parsed.Error != nil {
		return nil, serverError{code: parsed.Error.Code, msg: parsed.Error.Message}
	}
	return parsed.Result, nil
}
