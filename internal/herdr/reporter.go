// Package herdr reports the currently open agent thread to herdr's
// agent sidebar over herdr's socket API: one fresh
// connection per call, newline-delimited JSON requests, one JSON
// response line per request (read best-effort, contents ignored).
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

	wg sync.WaitGroup

	mu sync.Mutex
	// agent is the id from the last Report, carried into Release because
	// pane.release_agent requires it. Empty means no entry to release.
	agent string
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
	if addr := os.Getenv("SLK_HERDR_ADDR"); addr != "" {
		return newReporter("tcp", addr, paneID)
	}
	if path := os.Getenv("HERDR_SOCKET_PATH"); path != "" {
		return newReporter("unix", path, paneID)
	}
	return nil
}

func newReporter(network, addr, paneID string) *Reporter {
	return &Reporter{network: network, addr: addr, paneID: paneID}
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

// send delivers reqs over one fresh connection on its own goroutine, so
// callers (the UI goroutine) never block on the socket. Failures are logged
// and dropped: the sidebar is best-effort, and the next Report supersedes
// anything lost.
func (r *Reporter) send(reqs ...request) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, req := range reqs {
		if err := enc.Encode(req); err != nil {
			debuglog.Notify("herdr: encode %s: %v", req.Method, err)
			return
		}
	}
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		conn, err := net.DialTimeout(r.network, r.addr, sendTimeout)
		if err != nil {
			debuglog.Notify("herdr: dial %s %s: %v", r.network, r.addr, err)
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(sendTimeout))
		if _, err := conn.Write(buf.Bytes()); err != nil {
			debuglog.Notify("herdr: write: %v", err)
			return
		}
		scanner := bufio.NewScanner(conn)
		for range reqs {
			if !scanner.Scan() {
				return
			}
		}
	}()
}
