package herdr

import (
	"strings"
	"testing"
)

const paneOpenURL = "https://myteam.slack.com/archives/C1/p1779284733270139?thread_ts=1779284733.270139&cid=C1"

func TestOpenPane(t *testing.T) {
	r, rec := tabOpenReporter(t)

	if _, err := r.OpenPane("", "slk", paneOpenURL, true); err != nil {
		t.Fatal(err)
	}

	// No pane.report_metadata between split and wait: the new pane
	// shares slk's tab and must not claim its label.
	lines := waitLines(t, rec, 3)
	if len(lines) != 3 {
		t.Fatalf("lines = %v, want split, wait_for_output, send_input", lines)
	}
	method, params := decode(t, lines[0])
	if method != "pane.split" {
		t.Fatalf("first method = %q, want pane.split", method)
	}
	if params["direction"] != "right" || params["target_pane_id"] != "pane-1" || params["focus"] != true {
		t.Errorf("pane.split params = %v", params)
	}

	method, params = decode(t, lines[1])
	if method != "pane.wait_for_output" || params["pane_id"] != "w1:p10" {
		t.Errorf("second request = %s %v, want pane.wait_for_output on the new pane", method, params)
	}

	method, params = decode(t, lines[2])
	if method != "pane.send_input" || params["pane_id"] != "w1:p10" {
		t.Fatalf("third request = %s %v, want pane.send_input on the new pane", method, params)
	}
	if params["text"] != "slk '"+paneOpenURL+"'" {
		t.Errorf("send_input text = %q", params["text"])
	}
	keys, _ := params["keys"].([]any)
	if len(keys) != 1 || keys[0] != "Enter" {
		t.Errorf("send_input keys = %v, want [Enter]", params["keys"])
	}
}

func TestOpenPane_Unfocused(t *testing.T) {
	r, rec := tabOpenReporter(t)

	if _, err := r.OpenPane("", "slk", paneOpenURL, false); err != nil {
		t.Fatal(err)
	}
	method, params := decode(t, waitLines(t, rec, 3)[0])
	if method != "pane.split" || params["focus"] != false {
		t.Errorf("first request = %s %v, want pane.split with focus false", method, params)
	}
}

// The split targets the pane id slk holds now, which the focus watcher
// updates after a move, not the one from the launch env.
func TestOpenPane_TargetsCurrentPaneID(t *testing.T) {
	r, rec := tabOpenReporter(t)
	r.adopt(paneInfo{PaneID: "w1:p3"})

	if _, err := r.OpenPane("", "slk", paneOpenURL, true); err != nil {
		t.Fatal(err)
	}
	if _, params := decode(t, waitLines(t, rec, 3)[0]); params["target_pane_id"] != "w1:p3" {
		t.Errorf("target_pane_id = %v, want the adopted w1:p3", params["target_pane_id"])
	}
}

// A batch splits the pane its previous link opened: a given splitPaneID
// is the split target, and the new pane's id comes back for the next.
func TestOpenPane_SplitsGivenPane_ReturnsNewID(t *testing.T) {
	r, rec := tabOpenReporter(t)

	id, err := r.OpenPane("w1:p7", "slk", paneOpenURL, false)
	if err != nil {
		t.Fatal(err)
	}
	if id != "w1:p10" {
		t.Errorf("returned id = %q, want the new pane w1:p10", id)
	}
	if _, params := decode(t, waitLines(t, rec, 3)[0]); params["target_pane_id"] != "w1:p7" {
		t.Errorf("target_pane_id = %v, want the given w1:p7", params["target_pane_id"])
	}
}

func TestOpenPane_SplitError_NoInputSent(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodError("pane.split", "pane_not_found")

	_, err := r.OpenPane("", "slk", paneOpenURL, true)
	if err == nil || !strings.Contains(err.Error(), "pane_not_found") {
		t.Fatalf("err = %v, want the server's message", err)
	}
	if lines := rec.snapshot(); len(lines) != 1 {
		t.Errorf("lines = %v, want only the failed pane.split", lines)
	}
}

// A pane.send_input failure rolls the just-split pane back with
// pane.close, so a failed open leaves no empty pane beside slk.
func TestOpenPane_SendInputError_ClosesPane(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodError("pane.send_input", "pane gone")

	_, err := r.OpenPane("", "slk", paneOpenURL, true)
	if err == nil || !strings.Contains(err.Error(), "pane gone") {
		t.Fatalf("err = %v, want the send_input message", err)
	}
	method, params := decode(t, waitLines(t, rec, 4)[3])
	if method != "pane.close" || params["pane_id"] != "w1:p10" {
		t.Errorf("last request = %s %v, want pane.close of the new pane", method, params)
	}
}

// A transport failure on pane.send_input (no reply) must NOT close the
// pane: the input may have been delivered.
func TestOpenPane_SendInputTransportFailure_KeepsPane(t *testing.T) {
	r, rec := tabOpenReporter(t)
	rec.setMethodSilent("pane.send_input")

	if _, err := r.OpenPane("", "slk", paneOpenURL, true); err == nil {
		t.Fatal("expected an error")
	}
	for _, line := range rec.snapshot() {
		if method, _ := decode(t, line); method == "pane.close" {
			t.Error("pane.close sent after a transport failure")
		}
	}
}
