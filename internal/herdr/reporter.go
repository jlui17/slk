// Package herdr reports the currently open agent thread to herdr's
// agent sidebar over herdr's socket API: one fresh connection per
// request — the server handles a single newline-delimited JSON request
// per connection and silently discards pipelined extras — with one JSON
// response line each (read best-effort, contents ignored).
package herdr

import (
	"bufio"
	"bytes"
	"encoding/json"
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

// Reporter sends agent-sidebar updates for one herdr pane. Every method
// returns immediately (sends run on their own goroutine, ordered on the
// herdr side by a seq captured at call time) and is safe on a nil
// receiver, so callers outside a herdr pane need no guards.
type Reporter struct {
	network string
	addr    string
	paneID  string
	tabID   string

	wg sync.WaitGroup

	mu sync.Mutex
	// agent is the id from the last Report, carried into Release because
	// pane.release_agent requires it. Empty means no entry to release.
	agent string

	// tabMu serializes NameTab's read-then-rename sequence, and is separate
	// from mu so a Report on the UI goroutine never waits behind NameTab's
	// socket round-trips.
	tabMu sync.Mutex
}

// NewReporterFromEnv returns a Reporter when slk is running inside a herdr
// pane, else nil. Requires HERDR_ENV=1 and HERDR_PANE_ID. The address comes
// from SLK_HERDR_ADDR ("host:port", TCP — used when slk runs in a container
// and a host-side bridge forwards to the socket) or else HERDR_SOCKET_PATH
// (unix socket path).
func NewReporterFromEnv() *Reporter {
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
	if addr := os.Getenv("SLK_HERDR_ADDR"); addr != "" {
		return newReporter("tcp", addr, paneID, tabID)
	}
	if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
		return newReporter("unix", path, paneID, tabID)
	}
	return nil
}

func newReporter(network, addr, paneID, tabID string) *Reporter {
	return &Reporter{network: network, addr: addr, paneID: paneID, tabID: tabID}
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
// sidebar (e.g. "Claude"), title the pane title (channel + thread snippet),
// working the state (true → "working", false → "idle"), statusMessage the
// transient status text ("is thinking…", may be empty).
func (r *Reporter) Report(agent, displayName, title string, working bool, statusMessage string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.agent = agent
	r.mu.Unlock()
	state := "idle"
	if working {
		state = "working"
	}
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
				PaneID:  r.paneID,
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
				PaneID:       r.paneID,
				Source:       source,
				DisplayAgent: displayName,
				Title:        title,
				Seq:          metaSeq,
			},
		},
	)
}

// Release removes this pane's agent-sidebar entry. A no-op until the first
// Report, since no entry exists to remove.
func (r *Reporter) Release() {
	if r == nil {
		return
	}
	r.mu.Lock()
	agent := r.agent
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
		Params: releaseAgentParams{PaneID: r.paneID, Source: source, Agent: agent, Seq: seq},
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

// NameTab renames the pane's herdr tab to label — but only over a label
// herdr assigned by default (the tab's position digits) or one NameTab set
// itself, tracked via a pane-metadata token; a label the user typed is
// never overwritten. No-op without a tab id (HERDR_TAB_ID absent) and
// nil-safe.
func (r *Reporter) NameTab(label string) {
	if r == nil || r.tabID == "" || label == "" {
		return
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		r.tabMu.Lock()
		defer r.tabMu.Unlock()
		current, err := r.tabLabel()
		if err != nil {
			debuglog.Notify("herdr: tab.get: %v", err)
			return
		}
		if current == label {
			// Already named, possibly by a previous slk run in this pane;
			// claim it so later renames keep working.
			r.recordTabLabel(label)
			return
		}
		owned, err := r.ownsTabLabel(current)
		if err != nil {
			debuglog.Notify("herdr: pane.get: %v", err)
			return
		}
		if !owned && !isDefaultTabLabel(current) {
			return
		}
		req, err := json.Marshal(request{
			ID:     fmt.Sprintf("slk:%d", nextSeq()),
			Method: "tab.rename",
			Params: tabRenameParams{TabID: r.tabID, Label: label},
		})
		if err != nil {
			debuglog.Notify("herdr: encode tab.rename: %v", err)
			return
		}
		if _, err := r.deliver(append(req, '\n')); err != nil {
			debuglog.Notify("herdr: tab.rename: %v", err)
			return
		}
		r.recordTabLabel(label)
	}()
}

// ownsTabLabel reports whether current is a label NameTab set: the pane's
// ownership token matches it.
func (r *Reporter) ownsTabLabel(current string) (bool, error) {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: "pane.get",
		Params: paneGetParams{PaneID: r.paneID},
	})
	if err != nil {
		return false, err
	}
	resp, err := r.deliver(append(req, '\n'))
	if err != nil {
		return false, err
	}
	var parsed struct {
		Result struct {
			Pane struct {
				Tokens map[string]string `json:"tokens"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return false, err
	}
	return parsed.Result.Pane.Tokens[tabLabelToken] == current, nil
}

// recordTabLabel stores the ownership token; failures only cost a future
// rename, so they are logged and dropped.
func (r *Reporter) recordTabLabel(label string) {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", nextSeq()),
		Method: "pane.report_metadata",
		Params: reportTokensParams{
			PaneID: r.paneID,
			Source: source,
			Tokens: map[string]string{tabLabelToken: label},
			Seq:    nextSeq(),
		},
	})
	if err != nil {
		debuglog.Notify("herdr: encode token write: %v", err)
		return
	}
	if _, err := r.deliver(append(req, '\n')); err != nil {
		debuglog.Notify("herdr: token write: %v", err)
	}
}

// tabLabel fetches the tab's current label via tab.get.
func (r *Reporter) tabLabel() (string, error) {
	req, err := json.Marshal(request{
		ID:     fmt.Sprintf("slk:%d", time.Now().UnixNano()),
		Method: "tab.get",
		Params: tabGetParams{TabID: r.tabID},
	})
	if err != nil {
		return "", err
	}
	resp, err := r.deliver(append(req, '\n'))
	if err != nil {
		return "", err
	}
	var parsed struct {
		Result struct {
			Tab struct {
				Label string `json:"label"`
			} `json:"tab"`
		} `json:"result"`
	}
	if err := json.Unmarshal(resp, &parsed); err != nil {
		return "", err
	}
	return parsed.Result.Tab.Label, nil
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

// Close releases the entry and waits up to timeout for in-flight sends to
// finish; called once at process shutdown.
func (r *Reporter) Close(timeout time.Duration) {
	if r == nil {
		return
	}
	r.Release()
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
// reqs from a single call stay ordered. Failures are logged and dropped:
// the sidebar is best-effort, and the next Report supersedes anything lost.
func (r *Reporter) send(reqs ...request) {
	bufs := make([][]byte, 0, len(reqs))
	for _, req := range reqs {
		var buf bytes.Buffer
		if err := json.NewEncoder(&buf).Encode(req); err != nil {
			debuglog.Notify("herdr: encode %s: %v", req.Method, err)
			return
		}
		bufs = append(bufs, buf.Bytes())
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		for i, b := range bufs {
			if _, err := r.deliver(b); err != nil {
				debuglog.Notify("herdr: %s: %v", reqs[i].Method, err)
			}
		}
	}()
}

// deliver sends one request line over a fresh connection and returns the
// single response line (empty when the server closes without replying).
func (r *Reporter) deliver(line []byte) ([]byte, error) {
	conn, err := net.DialTimeout(r.network, r.addr, sendTimeout)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(sendTimeout))
	if _, err := conn.Write(line); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return nil, scanner.Err()
	}
	return scanner.Bytes(), nil
}
