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
	mu    sync.Mutex
	lines []string
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

// startServer runs a fake herdr endpoint that records each request line and
// replies with one ok-result line per request.
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
				for scanner.Scan() {
					rec.add(scanner.Text())
					conn.Write([]byte(`{"id":"x","result":{"type":"ok"}}` + "\n"))
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

func TestReport(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1")

	r.Report("slack-claude", "Claude", "#general · thread", true, "is thinking…")
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
		"seq":           seq,
	}
	for k, v := range want {
		if params[k] != v {
			t.Errorf("report_metadata params[%q] = %v, want %v", k, params[k], v)
		}
	}
}

func TestReportIdleOmitsEmptyMessage(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1")

	r.Report("slack-claude", "Claude", "title", false, "")
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
	r := newReporter("tcp", ln.Addr().String(), "pane-1")

	r.Report("slack-claude", "Claude", "title", true, "")
	lines := waitLines(t, rec, 2)
	if method, _ := decode(t, lines[0]); method != "pane.report_agent" {
		t.Fatalf("first method = %q, want pane.report_agent", method)
	}
}

func TestRelease(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1")

	r.Report("slack-claude", "Claude", "title", true, "")
	waitLines(t, rec, 2)
	r.Release()
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
}

func TestReleaseBeforeReportSendsNothing(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1")

	r.Release()
	r.Close(time.Second)
	if lines := rec.snapshot(); len(lines) != 0 {
		t.Fatalf("expected no requests, got %v", lines)
	}
}

func TestNilReporter(t *testing.T) {
	var r *Reporter
	r.Report("a", "A", "t", true, "m")
	r.Release()
	r.Close(time.Second)
}

func TestCloseWaitsForInFlight(t *testing.T) {
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1")

	r.Report("slack-claude", "Claude", "title", true, "")
	r.Close(2 * time.Second)

	// Close released and waited, so all three requests must already be
	// recorded. The Report and Release sends run on concurrent goroutines
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
