package herdr

import (
	"bufio"
	"encoding/json"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type recorder struct {
	mu            sync.Mutex
	lines         []string
	tabLabel      string
	tokens        map[string]string
	methodErrors  map[string]string
	silentMethods map[string]bool
}

// setMethodSilent makes the fake server close method's connection
// without replying (a transport failure from the client's view).
func (r *recorder) setMethodSilent(method string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.silentMethods == nil {
		r.silentMethods = map[string]bool{}
	}
	r.silentMethods[method] = true
}

func (r *recorder) methodSilent(method string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.silentMethods[method]
}

// setMethodError makes the fake server answer method with an error
// response carrying msg.
func (r *recorder) setMethodError(method, msg string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.methodErrors == nil {
		r.methodErrors = map[string]string{}
	}
	r.methodErrors[method] = msg
}

func (r *recorder) methodError(method string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.methodErrors[method]
}

func (r *recorder) add(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
}

func (r *recorder) snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.lines...)
}

func (r *recorder) setTabLabel(label string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tabLabel = label
}

func (r *recorder) getTabLabel() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.tabLabel
}

func (r *recorder) setToken(k, v string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tokens == nil {
		r.tokens = map[string]string{}
	}
	r.tokens[k] = v
}

func (r *recorder) getTokens() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]string{}
	for k, v := range r.tokens {
		out[k] = v
	}
	return out
}

// startServer runs a fake herdr endpoint that mirrors the real server's
// connection contract: exactly one request per connection — the first line
// is recorded and answered, then the connection closes, so a client that
// pipelines a second request on the same connection loses it.
func startServer(t *testing.T, network, addr string) (net.Listener, *recorder) {
	t.Helper()
	ln, err := net.Listen(network, addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	rec := &recorder{}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				scanner := bufio.NewScanner(conn)
				if scanner.Scan() {
					line := scanner.Text()
					rec.add(line)
					var req struct {
						Method string `json:"method"`
						Params struct {
							Label  string            `json:"label"`
							Tokens map[string]string `json:"tokens"`
						} `json:"params"`
					}
					_ = json.Unmarshal([]byte(line), &req)
					if rec.methodSilent(req.Method) {
						return
					}
					if msg := rec.methodError(req.Method); msg != "" {
						resp, _ := json.Marshal(map[string]any{
							"id":    "x",
							"error": map[string]any{"code": "boom", "message": msg},
						})
						conn.Write(append(resp, '\n'))
						return
					}
					switch req.Method {
					case "tab.get":
						resp, _ := json.Marshal(map[string]any{
							"id": "x",
							"result": map[string]any{
								"type": "tab_info",
								"tab":  map[string]any{"label": rec.getTabLabel()},
							},
						})
						conn.Write(append(resp, '\n'))
					case "tab.rename":
						rec.setTabLabel(req.Params.Label)
						conn.Write([]byte(`{"id":"x","result":{"type":"ok"}}` + "\n"))
					case "pane.get":
						resp, _ := json.Marshal(map[string]any{
							"id": "x",
							"result": map[string]any{
								"type": "pane_info",
								"pane": map[string]any{"tokens": rec.getTokens()},
							},
						})
						conn.Write(append(resp, '\n'))
					case "pane.report_metadata":
						for k, v := range req.Params.Tokens {
							rec.setToken(k, v)
						}
						conn.Write([]byte(`{"id":"x","result":{"type":"ok"}}` + "\n"))
					default:
						if req.Method == "tab.create" {
							conn.Write([]byte(`{"id":"x","result":{"type":"tab_created","tab":{"tab_id":"w1:t9"},"root_pane":{"pane_id":"w1:p9"}}}` + "\n"))
							break
						}
						conn.Write([]byte(`{"id":"x","result":{"type":"ok"}}` + "\n"))
					}
				}
			}()
		}
	}()
	return ln, rec
}

func waitLines(t *testing.T, rec *recorder, n int) []string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if lines := rec.snapshot(); len(lines) >= n {
			return lines
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lines, got %v", n, rec.snapshot())
	return nil
}

func decode(t *testing.T, line string) (method string, params map[string]any) {
	t.Helper()
	var req struct {
		ID     string         `json:"id"`
		Method string         `json:"method"`
		Params map[string]any `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		t.Fatalf("bad request line %q: %v", line, err)
	}
	if req.ID == "" {
		t.Errorf("request %q has empty id", line)
	}
	return req.Method, req.Params
}

func TestRequestsFollowAdoptedIdentity(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "w1:p1", "w1:t1")
	// What the focus watcher does after a cross-workspace move: every
	// request from here on must target the new coordinates, not the
	// launch env's.
	r.adopt(paneInfo{PaneID: "w2:p9", TabID: "w2:t9", WorkspaceID: "w2", TerminalID: "term_a"})

	r.Report("slack-claude", "Claude", "#eng fix retries", "working", "")
	for _, line := range waitLines(t, rec, 2) {
		method, params := decode(t, line)
		if params["pane_id"] != "w2:p9" {
			t.Errorf("%s pane_id = %v, want w2:p9", method, params["pane_id"])
		}
	}

	// tab.get, pane.get, tab.rename, token write — all on the new tab
	// and pane.
	r.NameTab("fix retries")
	for _, line := range waitLines(t, rec, 6)[2:] {
		method, params := decode(t, line)
		if id, has := params["tab_id"]; has && id != "w2:t9" {
			t.Errorf("%s tab_id = %v, want w2:t9", method, id)
		}
		if id, has := params["pane_id"]; has && id != "w2:p9" {
			t.Errorf("%s pane_id = %v, want w2:p9", method, id)
		}
	}

	if err := r.OpenTab("slk", "slk", "https://example.test"); err != nil {
		t.Fatalf("OpenTab: %v", err)
	}
	found := false
	for _, line := range rec.snapshot() {
		method, params := decode(t, line)
		if method != "tab.create" {
			continue
		}
		found = true
		if params["workspace_id"] != "w2" {
			t.Errorf("tab.create workspace_id = %v, want w2", params["workspace_id"])
		}
	}
	if !found {
		t.Error("OpenTab sent no tab.create")
	}

	before := len(rec.snapshot())
	r.release()
	method, params := decode(t, waitLines(t, rec, before+1)[before])
	if method != "pane.release_agent" || params["pane_id"] != "w2:p9" {
		t.Errorf("release: %s %v", method, params)
	}
}

func TestNameTabAbortsOnServerError(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	rec.setMethodError("tab.get", "tab w1:t1 not found")
	r := newReporter("unix", sock, "w1:p1", "w1:t1")

	// A tab.get error reply must abort the rename. Parsed as success it
	// reads as an empty label, which counts as a herdr default and lets
	// NameTab rename a tab it could not even read — exactly the state a
	// pane move or server restart leaves behind.
	r.NameTab("fix retries")
	r.Close(time.Second)
	for _, line := range rec.snapshot() {
		if method, _ := decode(t, line); method == "tab.rename" {
			t.Fatal("NameTab renamed despite tab.get failing")
		}
	}
}

func TestReport(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.Report("slack-claude", "Claude", "#general · thread", "working", "is thinking…")
	lines := waitLines(t, rec, 2)

	method, params := decode(t, lines[0])
	if method != "pane.report_agent" {
		t.Fatalf("first method = %q, want pane.report_agent", method)
	}
	want := map[string]any{
		"pane_id": "pane-1",
		"source":  "slk",
		"agent":   "slack-claude",
		"state":   "working",
		"message": "is thinking…",
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("report_agent params[%q] = %v, want %v", k, params[k], v)
		}
	}
	seq, ok := params["seq"].(float64)
	if !ok || seq <= 0 {
		t.Errorf("report_agent seq = %v, want positive number", params["seq"])
	}

	method, params = decode(t, lines[1])
	if method != "pane.report_metadata" {
		t.Fatalf("second method = %q, want pane.report_metadata", method)
	}
	want = map[string]any{
		"pane_id":       "pane-1",
		"source":        "slk",
		"display_agent": "Claude",
		"title":         "#general · thread",
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("report_metadata params[%q] = %v, want %v", k, params[k], v)
		}
	}
	// Strictly past the agent report's seq — herdr's per-pane seq counter
	// is shared across report methods and drops an equal-or-stale seq.
	// Compared as int64: UnixNano exceeds float64's exact-integer range,
	// so the map[string]any float values collapse adjacent seqs.
	if agentSeq, metaSeq := seqOf(t, lines[0]), seqOf(t, lines[1]); metaSeq <= agentSeq {
		t.Errorf("report_metadata seq = %d, want > %d", metaSeq, agentSeq)
	}
}

func seqOf(t *testing.T, line string) int64 {
	t.Helper()
	var req struct {
		Params struct {
			Seq int64 `json:"seq"`
		} `json:"params"`
	}
	if err := json.Unmarshal([]byte(line), &req); err != nil {
		t.Fatalf("bad request line %q: %v", line, err)
	}
	return req.Params.Seq
}

func TestNextSeqMonotonic(t *testing.T) {
	prev := nextSeq()
	for i := 0; i < 1000; i++ {
		next := nextSeq()
		if next <= prev {
			t.Fatalf("seq regressed: %d then %d", prev, next)
		}
		prev = next
	}
}

func TestReportIdleOmitsEmptyMessage(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.Report("slack-claude", "Claude", "title", "idle", "")
	lines := waitLines(t, rec, 2)

	_, params := decode(t, lines[0])
	if params["state"] != "idle" {
		t.Errorf("state = %v, want idle", params["state"])
	}
	if _, present := params["message"]; present {
		t.Errorf("message key present with empty statusMessage: %v", params["message"])
	}
}

func TestReportTCP(t *testing.T) {
	ln, rec := startServer(t, "tcp", "127.0.0.1:0")
	r := newReporter("tcp", ln.Addr().String(), "pane-1", "tab-1")

	r.Report("slack-claude", "Claude", "title", "working", "")
	lines := waitLines(t, rec, 2)
	if method, _ := decode(t, lines[0]); method != "pane.report_agent" {
		t.Fatalf("first method = %q, want pane.report_agent", method)
	}
}

func TestRelease(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.Report("slack-claude", "Claude", "title", "working", "")
	waitLines(t, rec, 2)
	r.release()
	lines := waitLines(t, rec, 3)

	method, params := decode(t, lines[2])
	if method != "pane.release_agent" {
		t.Fatalf("method = %q, want pane.release_agent", method)
	}
	want := map[string]any{"pane_id": "pane-1", "source": "slk", "agent": "slack-claude"}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("release_agent params[%q] = %v, want %v", k, params[k], v)
		}
	}
	// A release without a fresh seq counts as 0, stale against the prior
	// report's UnixNano, and herdr silently ignores it.
	if seq, ok := params["seq"].(float64); !ok || seq <= 0 {
		t.Errorf("release_agent seq = %v, want positive number", params["seq"])
	}
}

func TestReleaseBeforeReportSendsNothing(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.release()
	r.Close(time.Second)
	if lines := rec.snapshot(); len(lines) != 0 {
		t.Fatalf("expected no requests, got %v", lines)
	}
}

func TestNilReporter(t *testing.T) {
	var r *Reporter
	r.Report("a", "A", "t", "working", "m")
	r.release()
	r.NameTab("x")
	r.Close(time.Second)
}

func TestNameTabRenamesDefaultAndOwnLabel(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	rec.setTabLabel("2") // herdr default: position digits
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.NameTab("fix ingest retries")
	r.Close(time.Second)
	if got := rec.getTabLabel(); got != "fix ingest retries" {
		t.Fatalf("default label not renamed: %q", got)
	}

	// A later agent thread replaces a name NameTab itself set.
	r.NameTab("review the deploy")
	r.Close(time.Second)
	if got := rec.getTabLabel(); got != "review the deploy" {
		t.Fatalf("own label not renamed: %q", got)
	}
}

func TestNameTabNeverOverwritesUserLabel(t *testing.T) {
	for _, userLabel := range []string{"my precious tab", "2024"} {
		sock := filepath.Join(t.TempDir(), "herdr.sock")
		_, rec := startServer(t, "unix", sock)
		rec.setTabLabel(userLabel)
		r := newReporter("unix", sock, "pane-1", "tab-1")

		r.NameTab("fix ingest retries")
		r.Close(time.Second)
		if got := rec.getTabLabel(); got != userLabel {
			t.Fatalf("user label %q overwritten: %q", userLabel, got)
		}
		for _, line := range rec.snapshot() {
			if method, _ := decode(t, line); method == "tab.rename" {
				t.Fatalf("tab.rename sent over user label %q", userLabel)
			}
		}
	}
}

func TestNameTabOwnershipSurvivesRestart(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	// A previous slk run named the tab and recorded the ownership token
	// in the herdr server.
	rec.setTabLabel("old thread name")
	rec.setToken(tabLabelToken, "old thread name")

	r := newReporter("unix", sock, "pane-1", "tab-1")
	r.NameTab("new thread name")
	r.Close(time.Second)
	if got := rec.getTabLabel(); got != "new thread name" {
		t.Fatalf("own label from a previous run not renamed: %q", got)
	}
	if got := rec.getTokens()[tabLabelToken]; got != "new thread name" {
		t.Fatalf("ownership token not updated: %q", got)
	}
}

func TestNameTabNoTabID(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "")

	r.NameTab("fix ingest retries")
	r.Close(time.Second)
	if lines := rec.snapshot(); len(lines) != 0 {
		t.Fatalf("expected no requests without a tab id, got %v", lines)
	}
}

func TestCloseWaitsForInFlight(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")

	r.Report("slack-claude", "Claude", "title", "working", "")
	r.Close(2 * time.Second)

	// Close released and waited, so all three requests must already be
	// recorded. The Report and release sends run on concurrent goroutines
	// over separate connections, so only the set is deterministic.
	lines := rec.snapshot()
	if len(lines) != 3 {
		t.Fatalf("after Close got %d lines, want 3: %v", len(lines), lines)
	}
	got := map[string]bool{}
	for _, line := range lines {
		method, _ := decode(t, line)
		got[method] = true
	}
	for _, method := range []string{"pane.report_agent", "pane.report_metadata", "pane.release_agent"} {
		if !got[method] {
			t.Errorf("missing request %s in %v", method, lines)
		}
	}
}
