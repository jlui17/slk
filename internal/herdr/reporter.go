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
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

const (
	source      = "slk"
	sendTimeout = 500 * time.Millisecond
)

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

	// tabMu serializes NameTab's read-then-rename pair, and is separate
	// from mu so a Report on the UI goroutine never waits behind NameTab's
	// socket round-trips. lastTabLabel is the label NameTab set most
	// recently, guarded by tabMu.
	tabMu        sync.Mutex
	lastTabLabel string
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
	seq := time.Now().UnixNano()
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
			ID:     fmt.Sprintf("slk:%d", seq+1),
			Method: "pane.report_metadata",
			Params: reportMetadataParams{
				PaneID:       r.paneID,
				Source:       source,
				DisplayAgent: displayName,
				Title:        title,
				// Past the agent report's seq: herdr's per-pane seq counter
				// is shared across report methods and silently drops an
				// equal-or-stale seq (returning ok), which would eat the
				// metadata every time.
				Seq: seq + 1,
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
	// missing seq counts as 0, stale against any prior report's UnixNano,
	// and herdr silently ignores the release (returning ok).
	seq := time.Now().UnixNano()
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

// NameTab renames the pane's herdr tab to label — but only over a label
// herdr assigned by default (the tab's position digits) or one NameTab
// itself set earlier; a label the user typed is never overwritten. No-op
// without a tab id (HERDR_TAB_ID absent) and nil-safe.
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
			r.lastTabLabel = label
			return
		}
		if current != r.lastTabLabel && !isDefaultTabLabel(current) {
			return
		}
		req, err := json.Marshal(request{
			ID:     fmt.Sprintf("slk:%d", time.Now().UnixNano()),
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
		r.lastTabLabel = label
	}()
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
// unnamed tab's label is its position rendered as digits.
func isDefaultTabLabel(label string) bool {
	if label == "" {
		return true
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
