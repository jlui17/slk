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
// the pane.get / pane.list / workspace.get location probes from state the
// test sets, and streams focus events to whichever subscription
// connection is live.
type focusServer struct {
	t *testing.T

	mu          sync.Mutex
	pane        paneInfo
	activeTabID string
	wsFocused   bool
	// stale ids answer pane.get with pane_not_found, modeling a server
	// restart that wiped the stale-id alias map.
	stale map[string]bool
	// strictGet makes pane.get resolve only the pane's exact current id
	// (and foreign entries), modeling a freshly restarted server with no
	// aliases at all — the state a cold-start recovery probes.
	strictGet bool
	// foreign records answer pane.get for their own id, modeling other
	// panes; every other id resolves to pane, herdr's alias semantics.
	foreign map[string]paneInfo
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
		t:    t,
		pane: paneInfo{PaneID: "w1:p1", TabID: "w1:t1", WorkspaceID: "w1", TerminalID: "term_a"},
		activeTabID: "w1:t1", wsFocused: true,
		stale:      map[string]bool{},
		foreign:    map[string]paneInfo{},
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
		Params struct {
			PaneID string `json:"pane_id"`
		} `json:"params"`
	}
	_ = json.Unmarshal(scanner.Bytes(), &req)
	switch req.Method {
	case "pane.get":
		s.mu.Lock()
		var resp []byte
		if s.stale[req.Params.PaneID] ||
			(s.strictGet && req.Params.PaneID != s.pane.PaneID) {
			resp, _ = json.Marshal(map[string]any{"id": "x", "error": map[string]any{
				"code": "pane_not_found", "message": "pane " + req.Params.PaneID + " not found",
			}})
		} else if foreign, ok := s.foreign[req.Params.PaneID]; ok {
			resp, _ = json.Marshal(map[string]any{"id": "x", "result": map[string]any{
				"type": "pane_info",
				"pane": foreign,
			}})
		} else {
			resp, _ = json.Marshal(map[string]any{"id": "x", "result": map[string]any{
				"type": "pane_info",
				"pane": s.pane,
			}})
		}
		s.mu.Unlock()
		conn.Write(append(resp, '\n'))
	case "pane.list":
		s.mu.Lock()
		resp, _ := json.Marshal(map[string]any{"id": "x", "result": map[string]any{
			"type":  "pane_list",
			"panes": []paneInfo{s.pane},
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
	s.pane.TabID, s.activeTabID, s.wsFocused = tabID, activeTabID, wsFocused
}

// setPane replaces the pane record the probes report, for tests that
// move the pane; the terminal id stays, as it does across real moves.
func (s *focusServer) setPane(paneID, tabID, workspaceID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pane.PaneID, s.pane.TabID, s.pane.WorkspaceID = paneID, tabID, workspaceID
}

// markStale makes pane.get answer pane_not_found for id, modeling a
// restarted server that no longer aliases a moved pane's old id.
func (s *focusServer) markStale(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stale[id] = true
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
	r.WatchFocus(func(viewed bool) { views <- viewed }, nil)
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
	// that will never deliver an event and never retries. Acceptance is
	// caught as a view report arriving before any retry subscribed; the
	// report the retry's healthy connection sends is legitimate.
	r.WatchFocus(func(bool) {
		if s.subscribeCount() < 2 {
			t.Error("must not fire on a refused subscription")
		}
	}, nil)
	t.Cleanup(func() { r.Close(time.Second) })

	s.waitSubscribed() // the retry, after the bad ack was rejected
	if got := s.subscribeCount(); got < 2 {
		t.Fatalf("watcher did not retry after a bad ack; subscribes=%d", got)
	}
}

// dropSubscription ends the live subscription connection, forcing the
// watcher into its reconnect path.
func (s *focusServer) dropSubscription() {
	s.mu.Lock()
	ch := s.events
	s.events = nil
	s.mu.Unlock()
	if ch != nil {
		close(ch)
	}
}

func moveEvent(prev string, p paneInfo) map[string]any {
	return map[string]any{"event": "pane_moved", "data": map[string]any{
		"type": "pane_moved", "previous_pane_id": prev, "pane": p,
	}}
}

// waitIdentity polls until the reporter's tracked pane id becomes want.
func waitIdentity(t *testing.T, r *Reporter, want string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if r.identity().PaneID == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	id := r.identity()
	t.Fatalf("identity = %s/%s/%s, want pane %s", id.PaneID, id.TabID, id.WorkspaceID, want)
}

func TestWatchFocusTracksIdentityAcrossMoves(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	// First cross-workspace move: the pane gets a new public id, into an
	// unfocused workspace. The events carry no terminal id (an older
	// herdr), so only the pane-id comparisons can match.
	s.setPane("w2:p1", "w2:t1", "w2")
	s.setLocation("w2:t1", "w2:t1", false)
	s.send(moveEvent("w1:p1", paneInfo{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2"}))
	view(false, "moved to an unfocused workspace")
	waitIdentity(t, r, "w2:p1")

	// Second move: its event names only ids from after the first move.
	// Matching against the launch id would drop it here, freezing the
	// watcher on w2 forever.
	s.setPane("w3:p1", "w3:t1", "w3")
	s.setLocation("w3:t1", "w3:t1", true)
	s.send(moveEvent("w2:p1", paneInfo{PaneID: "w3:p1", TabID: "w3:t1", WorkspaceID: "w3"}))
	view(true, "second move into the focused workspace")
	waitIdentity(t, r, "w3:p1")
	if id := r.identity(); id.TabID != "w3:t1" || id.WorkspaceID != "w3" {
		t.Errorf("tab/workspace = %s/%s, want w3:t1/w3", id.TabID, id.WorkspaceID)
	}
}

func TestWatchFocusMatchesMoveByTerminalID(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	// A move event whose pane ids both differ from the tracked one (the
	// tracked id drifted, e.g. an earlier event was lost) still names the
	// pane's terminal: that match must trigger the relocate.
	s.setPane("w4:p2", "w4:t1", "w4")
	s.setLocation("w4:t1", "w4:t1", false)
	s.send(moveEvent("w9:p9", paneInfo{PaneID: "w4:p2", TabID: "w4:t1", WorkspaceID: "w4", TerminalID: "term_a"}))
	view(false, "relocated on terminal id alone")
	waitIdentity(t, r, "w4:p2")
}

func TestWatchFocusIgnoresForeignMovesWithoutTerminalIDs(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	// An older herdr that reports no terminal ids at all: the tracked
	// terminal id stays empty, and an empty event terminal id must not
	// count as a match — that would adopt every other pane's move.
	s.mu.Lock()
	s.pane.TerminalID = ""
	s.foreign["w9:p8"] = paneInfo{PaneID: "w9:p8", TabID: "w9:t1", WorkspaceID: "w9"}
	s.mu.Unlock()
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	view, quiet := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	s.send(moveEvent("w9:p9", paneInfo{PaneID: "w9:p8", TabID: "w9:t1", WorkspaceID: "w9"}))
	quiet("foreign pane's move")
	if id := r.identity(); id.PaneID != "w1:p1" {
		t.Fatalf("identity hijacked by a foreign move: %s", id.PaneID)
	}

	// The pane's own move still matches through the id fallback.
	s.setPane("w2:p1", "w2:t1", "w2")
	s.setLocation("w2:t1", "w2:t1", false)
	s.send(moveEvent("w1:p1", paneInfo{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2"}))
	view(false, "own move into an unfocused workspace")
	waitIdentity(t, r, "w2:p1")
}

func TestWatchFocusRecoversFromStalePaneID(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	r.retryDelay = 20 * time.Millisecond
	connects := make(chan struct{}, 8)
	views := make(chan bool, 8)
	r.WatchFocus(func(viewed bool) { views <- viewed }, func() { connects <- struct{}{} })
	t.Cleanup(func() { r.Close(time.Second) })
	waitSignal := func(step string) {
		t.Helper()
		select {
		case <-connects:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: no connect signal", step)
		}
	}
	s.waitSubscribed()
	waitSignal("initial connect")
	<-views // initial viewed state; the first pane.get learned term_a

	// The tracked id stops resolving while the pane's terminal id stays
	// current — a state no known herdr sequence produces (see fetchPane:
	// aliases cover stale ids within a run, restarts re-mint terminal
	// ids). This pins the fallback kept for the day that belief fails;
	// the real restart recovery is TestWatchFocusBootstrapsFromCachedPaneID.
	s.markStale("w1:p1")
	s.setPane("w5:p2", "w5:t1", "w5")
	s.setLocation("w5:t1", "w5:t1", true)
	s.dropSubscription()

	// The reconnect must fall back to pane.list by terminal id, adopt the
	// new id, and resubscribe.
	s.waitSubscribed()
	waitSignal("reconnect")
	waitIdentity(t, r, "w5:p2")
}

func TestWatchFocusBootstrapsFromCachedPaneID(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	// A cold start in the wedged pane: the launch env still says w1:p1
	// (the pane moved workspaces, then a restart wiped every alias),
	// no terminal id has ever been learned in this process, and the pane
	// answers only to its exact current id, w5:p2 — strictGet, so a
	// probe with any other id (the launch id, an empty string, a field
	// mixup) fails like it would against the real restarted server.
	// Without the cached id the first locate can never succeed and the
	// watcher never subscribes.
	s.mu.Lock()
	s.strictGet = true
	s.mu.Unlock()
	s.setPane("w5:p2", "w5:t1", "w5")
	s.setLocation("w5:t1", "w5:t1", true)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	r.SetPaneIDCache(func() (string, bool) { return "w5:p2", true }, func(string) error { return nil })
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "bootstrapped from the cached id")
	waitIdentity(t, r, "w5:p2")
}

func TestWatchFocusRetriesFailedPaneIDSave(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	r.retryDelay = 20 * time.Millisecond
	// The saver runs only on the watcher's goroutine, so the plain bool
	// is race-free. First write fails, as a busy cache DB's would.
	failedOnce := false
	saves := make(chan string, 8)
	r.SetPaneIDCache(nil, func(paneID string) error {
		if !failedOnce {
			failedOnce = true
			return fmt.Errorf("database is locked")
		}
		saves <- paneID
		return nil
	})
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	// A failed save must not count as saved: the next resolve (here a
	// reconnect landing on the same id) retries it.
	s.dropSubscription()
	s.waitSubscribed()
	select {
	case got := <-saves:
		if got != "w1:p1" {
			t.Fatalf("retried save = %q, want %q", got, "w1:p1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("failed save was never retried")
	}
}

func TestWatchFocusPersistsResolvedPaneID(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	r.retryDelay = 20 * time.Millisecond
	saves := make(chan string, 8)
	r.SetPaneIDCache(nil, func(paneID string) error {
		saves <- paneID
		return nil
	})
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	waitSave := func(want, step string) {
		t.Helper()
		select {
		case got := <-saves:
			if got != want {
				t.Fatalf("%s: saved %q, want %q", step, got, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s: nothing saved", step)
		}
	}
	// The first resolve persists even without a move: the row always
	// holds the last id the pane answered to, replacing whatever a
	// previous run left under this key. (A value equal to the launch id
	// is never read back — fetchPane skips it — so the write's job here
	// is scrubbing, not recovery.)
	waitSave("w1:p1", "initial resolve")

	s.setPane("w2:p1", "w2:t1", "w2")
	s.setLocation("w2:t1", "w2:t1", true)
	s.send(moveEvent("w1:p1", paneInfo{PaneID: "w2:p1", TabID: "w2:t1", WorkspaceID: "w2"}))
	waitSave("w2:p1", "after the move")

	// A reconnect that resolves the same id must not rewrite the store.
	s.dropSubscription()
	s.waitSubscribed()
	waitIdentity(t, r, "w2:p1")
	select {
	case got := <-saves:
		t.Fatalf("reconnect without a move re-saved %q", got)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestReportsDuringMovesNoRace(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	s := startFocusServer(t, sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	view, _ := focusWatcher(t, r)
	s.waitSubscribed()
	view(true, "initial state")

	// Reports race the watcher's identity adoption; run under -race this
	// exercises the lock rather than merely asserting it exists.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 40; i++ {
			state := "idle"
			if i%2 == 0 {
				state = "working"
			}
			r.Report("slack-claude", "Claude", "#eng fix retries", state, "")
		}
	}()
	for i := 1; i <= 10; i++ {
		pane := fmt.Sprintf("w2:p%d", i)
		s.setPane(pane, "w2:t1", "w2")
		s.send(moveEvent("w1:p1", paneInfo{PaneID: pane, TabID: "w2:t1", WorkspaceID: "w2", TerminalID: "term_a"}))
	}
	<-done
}

func TestWatchFocusNilSafe(t *testing.T) {
	var r *Reporter
	r.SetPaneIDCache(func() (string, bool) { return "", false }, func(string) error { return nil })
	r.WatchFocus(func(bool) { t.Error("must not fire") }, func() { t.Error("must not connect") })
	time.Sleep(20 * time.Millisecond)
}
