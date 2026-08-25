package herdr

import (
	"path/filepath"
	"strings"
	"testing"
)

func tabOpenReporter(t *testing.T) (*Reporter, *recorder) {
	t.Helper()
	old := shellStartupDelay
	shellStartupDelay = 0
	t.Cleanup(func() { shellStartupDelay = old })
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	r := newReporter("unix", sock, "pane-1", "tab-1")
	r.workspaceID = "w1"
	return r, rec
}

func TestOpenTab(t *testing.T) {
	r, rec := tabOpenReporter(t)

	url := "https://myteam.slack.com/archives/C1/p1779284733270139?thread_ts=1779284733.270139&cid=C1"
	if err := r.OpenTab("#general", "slk", url); err != nil {
		t.Fatal(err)
	}

	lines := waitLines(t, rec, 4)
	method, params := decode(t, lines[0])
	if method != "tab.create" {
		t.Fatalf("first method = %q, want tab.create", method)
	}
	if params["workspace_id"] != "w1" || params["label"] != "#general" || params["focus"] != true {
		t.Errorf("tab.create params = %v", params)
	}

	// The label just written on the new tab is claimed for its root pane,
	// so the slk instance about to boot there may refine it.
	method, params = decode(t, lines[1])
	if method != "pane.report_metadata" {
		t.Fatalf("second method = %q, want pane.report_metadata", method)
	}
	if params["pane_id"] != "w1:p9" {
		t.Errorf("claim pane_id = %v, want the created tab's root pane", params["pane_id"])
	}
	if got := rec.getTokens()[tabLabelToken]; got != "#general" {
		t.Errorf("claimed label token = %q, want %q", got, "#general")
	}

	method, params = decode(t, lines[2])
	if method != "pane.wait_for_output" {
		t.Fatalf("third method = %q, want pane.wait_for_output", method)
	}
	if params["pane_id"] != "w1:p9" {
		t.Errorf("wait_for_output pane_id = %v, want the created tab's root pane", params["pane_id"])
	}

	method, params = decode(t, lines[3])
	if method != "pane.send_input" {
		t.Fatalf("fourth method = %q, want pane.send_input", method)
	}
	if params["pane_id"] != "w1:p9" {
		t.Errorf("send_input pane_id = %v, want the created tab's root pane", params["pane_id"])
	}
	// The URL is single-quoted: permalinks carry ?thread_ts=...&cid=...,
	// which the pane's shell would otherwise interpret.
	if params["text"] != "slk '"+url+"'" {
		t.Errorf("send_input text = %q", params["text"])
	}
	keys, _ := params["keys"].([]any)
	if len(keys) != 1 || keys[0] != "Enter" {
		t.Errorf("send_input keys = %v, want [Enter]", params["keys"])
	}
}

// A failing pane.wait_for_output (older herdr without the method, or a
// shell quiet past the timeout) degrades to the fallback delay: the
// command is still sent and the open still succeeds.
func TestOpenTab_WaitError_StillSends(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodError("pane.wait_for_output", "unknown method")

	if err := r.OpenTab("#general", "slk", "https://myteam.slack.com/archives/C1/p1779284733270139"); err != nil {
		t.Fatal(err)
	}
	lines := waitLines(t, rec, 4)
	if method, _ := decode(t, lines[3]); method != "pane.send_input" {
		t.Errorf("fourth method = %q, want pane.send_input", method)
	}
}

func TestOpenTab_CreateError_NoInputSent(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodError("tab.create", "workspace_not_found")

	err := r.OpenTab("#general", "slk", "https://myteam.slack.com/archives/C1/p1779284733270139")
	if err == nil || !strings.Contains(err.Error(), "workspace_not_found") {
		t.Fatalf("err = %v, want the server's message", err)
	}
	if lines := rec.snapshot(); len(lines) != 1 {
		t.Errorf("lines = %v, want only the failed tab.create", lines)
	}
}

// A pane.send_input failure rolls the just-created tab back with
// tab.close, so a failed open leaves no empty tab behind.
func TestOpenTab_SendInputError_ClosesTab(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodError("pane.send_input", "pane gone")

	err := r.OpenTab("#general", "slk", "https://myteam.slack.com/archives/C1/p1779284733270139")
	if err == nil || !strings.Contains(err.Error(), "pane gone") {
		t.Fatalf("err = %v, want the send_input message", err)
	}
	lines := waitLines(t, rec, 5)
	method, params := decode(t, lines[4])
	if method != "tab.close" || params["tab_id"] != "w1:t9" {
		t.Errorf("last request = %s %v, want tab.close of the created tab", method, params)
	}
}

// A transport failure on pane.send_input (no reply) must NOT roll the
// tab back: the input may have been delivered, and tab.close would
// kill a successful open.
func TestOpenTab_SendInputTransportFailure_KeepsTab(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodSilent("pane.send_input")

	err := r.OpenTab("#general", "slk", "https://myteam.slack.com/archives/C1/p1779284733270139")
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, line := range rec.snapshot() {
		if method, _ := decode(t, line); method == "tab.close" {
			t.Error("tab.close sent after a transport failure")
		}
	}
}

func TestOpenTab_NoWorkspaceID(t *testing.T) {
	r, rec := tabOpenReporter(t)
	r.workspaceID = ""

	if r.CanOpenTab() {
		t.Error("CanOpenTab = true without a workspace id")
	}
	if err := r.OpenTab("#general", "slk", "https://x.slack.com/archives/C1/p1"); err == nil {
		t.Error("OpenTab succeeded without a workspace id")
	}
	if lines := rec.snapshot(); len(lines) != 0 {
		t.Errorf("lines = %v, want none", lines)
	}
}
