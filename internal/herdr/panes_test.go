package herdr

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// paneServer is a fake herdr endpoint answering one request per
// connection from a fixed method → result table, recording every
// request it saw.
type paneServer struct {
	results map[string]string // method → result JSON; absent methods get an error response

	mu   sync.Mutex
	seen []string
}

func startPaneServer(t *testing.T, results map[string]string) (*paneServer, *Reporter) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	s := &paneServer{results: results}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s, newReporter("unix", sock, "w1:p1", "")
}

func (s *paneServer) serve(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return
	}
	line := scanner.Text()
	var req struct {
		Method string `json:"method"`
	}
	_ = json.Unmarshal([]byte(line), &req)
	s.mu.Lock()
	s.seen = append(s.seen, line)
	result, ok := s.results[req.Method]
	s.mu.Unlock()
	if !ok {
		conn.Write([]byte(`{"id":"x","error":{"code":"timeout","message":"timed out waiting for output match"}}` + "\n"))
		return
	}
	// One response line per request: the table's JSON may be wrapped.
	var resp bytes.Buffer
	if err := json.Compact(&resp, []byte(`{"id":"x","result":`+result+`}`)); err != nil {
		panic(err)
	}
	conn.Write(append(resp.Bytes(), '\n'))
}

// request returns the i-th request the server saw, decoded.
func (s *paneServer) request(t *testing.T, i int) (string, map[string]any) {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if i >= len(s.seen) {
		t.Fatalf("server saw %d requests, want index %d", len(s.seen), i)
	}
	return decode(t, s.seen[i])
}

func TestListPanesIsUnscoped(t *testing.T) {
	s, r := startPaneServer(t, map[string]string{
		"pane.list": `{"type":"pane_list","panes":[
			{"pane_id":"w1:p1","tab_id":"w1:t1","workspace_id":"w1"},
			{"pane_id":"w2:p7","tab_id":"w2:t3","workspace_id":"w2"}]}`,
	})
	panes, err := r.ListPanes()
	if err != nil {
		t.Fatal(err)
	}
	want := []Pane{{PaneID: "w1:p1", WorkspaceID: "w1"}, {PaneID: "w2:p7", WorkspaceID: "w2"}}
	if len(panes) != 2 || panes[0] != want[0] || panes[1] != want[1] {
		t.Errorf("panes = %+v, want %+v", panes, want)
	}
	method, params := s.request(t, 0)
	if method != "pane.list" {
		t.Errorf("method = %s", method)
	}
	if _, scoped := params["workspace_id"]; scoped {
		t.Errorf("pane.list must not be scoped to a workspace: %v", params)
	}
}

func TestPaneProcessInfoParsesForeground(t *testing.T) {
	s, r := startPaneServer(t, map[string]string{
		"pane.process_info": `{"type":"pane_process_info","process_info":{"pane_id":"w1:p1","shell_pid":100,
			"foreground_process_group_id":200,"foreground_processes":[
			{"pid":200,"name":"docker","argv":["docker","run"],"cmdline":"docker run --name slk-user-1 slk-go:1.26 /src/bin/slk-linux","cwd":"/x"}]}}`,
	})
	info, err := r.PaneProcessInfo("w1:p1")
	if err != nil {
		t.Fatal(err)
	}
	if info.ShellPID != 100 || len(info.Foreground) != 1 || info.Foreground[0].PID != 200 ||
		info.Foreground[0].Cmdline != "docker run --name slk-user-1 slk-go:1.26 /src/bin/slk-linux" {
		t.Errorf("info = %+v", info)
	}
	method, params := s.request(t, 0)
	if method != "pane.process_info" || params["pane_id"] != "w1:p1" {
		t.Errorf("request: %s %v", method, params)
	}
}

func TestRunInPaneTypesCommandAndEnter(t *testing.T) {
	s, r := startPaneServer(t, map[string]string{"pane.send_input": `{"type":"ok"}`})
	if err := r.RunInPane("w2:p7", "slk"); err != nil {
		t.Fatal(err)
	}
	method, params := s.request(t, 0)
	if method != "pane.send_input" || params["pane_id"] != "w2:p7" || params["text"] != "slk" {
		t.Errorf("request: %s %v", method, params)
	}
	keys, _ := params["keys"].([]any)
	if len(keys) != 1 || keys[0] != "Enter" {
		t.Errorf("keys = %v, want [Enter]", params["keys"])
	}
}

func TestWaitForOutputMatchesVisibleSubstring(t *testing.T) {
	s, r := startPaneServer(t, map[string]string{"pane.wait_for_output": `{"type":"output_matched","matched_line":"NORMAL"}`})
	if err := r.WaitForOutput("w2:p7", "NORMAL", 3*time.Second); err != nil {
		t.Fatal(err)
	}
	method, params := s.request(t, 0)
	if method != "pane.wait_for_output" || params["pane_id"] != "w2:p7" || params["source"] != "visible" {
		t.Errorf("request: %s %v", method, params)
	}
	match, _ := params["match"].(map[string]any)
	if match["type"] != "substring" || match["value"] != "NORMAL" {
		t.Errorf("match = %v", params["match"])
	}
	if params["timeout_ms"] != float64(3000) {
		t.Errorf("timeout_ms = %v, want 3000", params["timeout_ms"])
	}
}

func TestWaitForOutputSurfacesTimeout(t *testing.T) {
	_, r := startPaneServer(t, map[string]string{})
	err := r.WaitForOutput("w2:p7", "NORMAL", time.Second)
	if err == nil {
		t.Fatal("want herdr's timeout error")
	}
	var se serverError
	if !errors.As(err, &se) || se.code != "timeout" {
		t.Errorf("err = %v, want serverError timeout", err)
	}
}
