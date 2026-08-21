package herdr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestReportUnreadSendsCompletionPair(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "w1:p1", "")

	r.ReportUnread("slack-claude", "Claude", "#eng fix retries", "2 unread replies")
	lines := waitLines(t, rec, 3)

	method, params := decode(t, lines[0])
	if method != "pane.report_agent" || params["state"] != "working" {
		t.Fatalf("first request must report working: %s %v", method, params)
	}
	if _, has := params["message"]; has {
		t.Errorf("working report must not carry the unread message: %v", params)
	}
	workingSeq := requestSeq(t, lines[0])

	method, params = decode(t, lines[1])
	if method != "pane.report_agent" || params["state"] != "idle" {
		t.Fatalf("second request must report idle: %s %v", method, params)
	}
	if params["message"] != "2 unread replies" {
		t.Errorf("idle report message = %v", params["message"])
	}
	if idleSeq := requestSeq(t, lines[1]); idleSeq <= workingSeq {
		t.Errorf("idle seq %v must outrank working seq %v", idleSeq, workingSeq)
	}

	method, params = decode(t, lines[2])
	if method != "pane.report_metadata" || params["display_agent"] != "Claude" || params["title"] != "#eng fix retries" {
		t.Errorf("metadata request: %s %v", method, params)
	}

	// The completion counts as this pane's live entry: release must
	// target it.
	r.release()
	lines = waitLines(t, rec, 4)
	method, params = decode(t, lines[3])
	if method != "pane.release_agent" || params["agent"] != "slack-claude" {
		t.Errorf("release after ReportUnread: %s %v", method, params)
	}
}

// requestSeq extracts the seq embedded in a request's "slk:<seq>" id.
// The params seq can't be read back through decode: float64 drops the
// low bits of a nanosecond seq, collapsing consecutive values.
func requestSeq(t *testing.T, line string) int64 {
	t.Helper()
	var req struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		t.Fatalf("bad request line %q: %v", line, err)
	}
	var seq int64
	if _, err := fmt.Sscanf(req.ID, "slk:%d", &seq); err != nil {
		t.Fatalf("id %q does not embed a seq: %v", req.ID, err)
	}
	return seq
}

// focusServer is a fake herdr endpoint for the focus watcher: it answers
// the pane.get / workspace.get location probes from state the test sets,
// and streams focus events to whichever subscription connection is live.
type focusServer struct {
	t *testing.T

	mu          sync.Mutex
	tabID       string
	activeTabID string
	wsFocused   bool
	// events belongs to the live subscription and is replaced on every
	// new subscribe, so a dropped connection's reader can never consume
	// an event meant for its successor.
	events     chan map[string]any
	subscribes int
	badAckOnce bool

	subscribed chan struct{}
	// done outlives the test's connections so the bad-ack path can hold
	// one open, which is what makes accepting a bad ack fatal: the
	// watcher parks on a connection that never delivers an event.
	done chan struct{}
}

func startFocusServer(t *testing.T, sock string) *focusServer {
	t.Helper()
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &focusServer{
		t: t, tabID: "w1:t1", activeTabID: "w1:t1", wsFocused: true,
		subscribed: make(chan struct{}, 8),
		done:       make(chan struct{}),
	}
	t.Cleanup(func() { close(s.done) })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *focusServer) serve(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	var req struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal(scanner.Bytes(), &req)
	switch req.Method {
	case "pane.get":
		s.mu.Lock()
		resp, _ := json.Marshal(map[string]any{"id": "x", "result": map[string]any{
			"type": "pane_info",
			"pane": map[string]any{"tab_id": s.tabID, "workspace_id": "w1"},
		}})
		s.mu.Unlock()
		conn.Write(append(resp, '\n'))
	case "workspace.get":
		s.mu.Lock()
		resp, _ := json.Marshal(map[string]any{"id": "x", "result": map[string]any{
			"type":      "workspace_info",
			"workspace": map[string]any{"focused": s.wsFocused, "active_tab_id": s.activeTabID},
		}})
		s.mu.Unlock()
		conn.Write(append(resp, '\n'))
	case "events.subscribe":
		s.mu.Lock()
		s.subscribes++
		bad := s.badAckOnce
		s.badAckOnce = false
		events := make(chan map[string]any)
		s.events = events
		s.mu.Unlock()
		if bad {
			conn.Write([]byte(`{"id":"x","result":{"type":"pane_info"}}` + "\n"))
			<-s.done
			return
		}
		conn.Write([]byte(`{"id":"x","result":{"type":"subscription_started"}}` + "\n"))
		s.subscribed <- struct{}{}
		for ev := range events {
			line, _ := json.Marshal(ev)
			if _, err := conn.Write(append(line, '\n')); err != nil {
				return
			}
		}
	default:
		conn.Write([]byte(`{"id":"x","result":{"type":"ok"}}` + "\n"))
	}
}

// setLocation updates what the location probes report.
func (s *focusServer) setLocation(tabID, activeTabID string, wsFocused bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tabID, s.activeTabID, s.wsFocused = tabID, activeTabID, wsFocused
}

func (s *focusServer) send(ev map[string]any) {
	s.mu.Lock()
	ch := s.events
	s.mu.Unlock()
	ch <- ev
}

// waitSubscribed blocks until a subscription connection is live.
func (s *focusServer) waitSubscribed() {
	s.t.Helper()
	select {
	case <-s.subscribed:
	case <-time.After(2 * time.Second):
		s.t.Fatal("watcher never subscribed")
	}
}

func (s *focusServer) subscribeCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subscribes
}

func focusEvent(kind, tabID, workspaceID string) map[string]any {
	data := map[string]any{"type": kind, "workspace_id": workspaceID}
	if tabID != "" {
		data["tab_id"] = tabID
	}
	return map[string]any{"event": kind, "data": data}
}

// focusWatcher starts r's watcher and returns two checkers over the
// viewed states it reports: one asserting the next report's value, one
// asserting no report arrives.
func focusWatcher(t *testing.T, r *Reporter) (view func(want bool, step string), quiet func(step string)) {
	t.Helper()
	views := make(chan bool, 8)
	r.WatchFocus(func(viewed bool) { views <- viewed })
	t.Cleanup(func() { r.Close(time.Second) })
	view = func(want bool, step string) {
		t.Helper()
		select {
		case got := <-views:
			if got != want {
				t.Fatalf("%s: reported viewed=%v, want %v", step, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s: no view report", step)
		}
	}
	quiet = func(step string) {
		t.Helper()
		select {
		case got := <-views:
			t.Fatalf("%s: unexpected view report %v", step, got)
		case <-time.After(300 * time.Millisecond):
		}
	}
	return view, quiet
}

func TestWatchFocusFiresOnUnfocusTransitions(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	view, quiet := focusWatcher(t, newReporter("unix", sock, "w1:p1", ""))
	s.waitSubscribed()
	view(true, "initial state")

	// Another workspace's internal tab switch leaves our own focus alone.
	s.send(focusEvent("tab_focused", "w2:t9", "w2"))
	quiet("other workspace tab switch")

	// Our workspace loses focus: viewed -> unviewed, reported once.
	s.send(focusEvent("workspace_focused", "", "w2"))
	view(false, "workspace unfocused")

	// Still unviewed: more far-side churn is not a transition.
	s.send(focusEvent("tab_focused", "w2:t8", "w2"))
	quiet("churn while unviewed")

	// Back to viewed, then our workspace switches to a sibling tab.
	s.send(focusEvent("workspace_focused", "", "w1"))
	view(true, "refocused")
	s.send(focusEvent("tab_focused", "w1:t2", "w1"))
	view(false, "sibling tab focused")
}

func TestWatchFocusSeedsFromLiveLocation(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	// The pane's tab is not the active one when the watcher connects, so
	// it must report unviewed up front. Assuming viewed (what a fresh
	// connection used to do) both misreports the state to consumers that
	// gate on it and inverts the fold, swallowing the next real unfocus.
	s.setLocation("w1:t1", "w1:t7", true)
	view, quiet := focusWatcher(t, newReporter("unix", sock, "w1:p1", ""))
	s.waitSubscribed()
	view(false, "seeded from the live location")

	s.send(focusEvent("workspace_focused", "", "w2"))
	quiet("already unviewed at seed time")

	// Coming into view and leaving again still reports both ways.
	s.send(focusEvent("workspace_focused", "", "w1"))
	s.send(focusEvent("tab_focused", "w1:t1", "w1"))
	view(true, "now viewed")
	s.send(focusEvent("tab_focused", "w1:t9", "w1"))
	view(false, "left the tab")
}

func TestWatchFocusTracksPaneMove(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	view, _ := focusWatcher(t, newReporter("unix", sock, "w1:p1", ""))
	s.waitSubscribed()
	view(true, "initial state")

	// herdr moves the pane out of the viewed tab and into one that isn't
	// active. That move is itself a viewed -> unviewed transition, and
	// only a re-resolve can see it: folding against the tab the pane left
	// leaves the watcher believing it is still viewed forever.
	s.setLocation("w1:t5", "w1:t6", true)
	s.send(map[string]any{"event": "pane_moved", "data": map[string]any{
		"type": "pane_moved", "previous_pane_id": "w1:p1",
		"pane": map[string]any{"pane_id": "w1:p1", "tab_id": "w1:t5", "workspace_id": "w1"},
	}})
	view(false, "moved out of the viewed tab")

	// Focus follows the pane to its new tab, which it cannot see if it is
	// still tracking the tab the pane left.
	s.send(focusEvent("tab_focused", "w1:t5", "w1"))
	view(true, "new tab focused")
	s.send(focusEvent("tab_focused", "w1:t9", "w1"))
	view(false, "left the tab the pane moved to")
}

func TestWatchFocusRejectsBadAckAndRetries(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	s.mu.Lock()
	s.badAckOnce = true
	s.mu.Unlock()
	r := newReporter("unix", sock, "w1:p1", "")
	r.retryDelay = 20 * time.Millisecond
	// A line that isn't a subscription_started ack must be rejected, not
	// taken for success: accepting it parks the watcher on a connection
	// that will never deliver an event and never retries.
	r.WatchFocus(func(bool) { t.Error("must not fire on a refused subscription") })
	t.Cleanup(func() { r.Close(time.Second) })

	s.waitSubscribed() // the retry, after the bad ack was rejected
	if got := s.subscribeCount(); got < 2 {
		t.Fatalf("watcher did not retry after a bad ack; subscribes=%d", got)
	}
}

func TestWatchFocusNilSafe(t *testing.T) {
	var r *Reporter
	r.WatchFocus(func(bool) { t.Error("must not fire") })
	time.Sleep(20 * time.Millisecond)
}
