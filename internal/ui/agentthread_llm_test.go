package ui

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

type relabelCall struct {
	teamID         string
	channelID      string
	threadTS       string
	transcript     string
	fallbackTaskID string
}

// newLLMLabelTestApp is an agent test app with the model-label generator
// installed, returning the generator capture and the NameTab capture.
func newLLMLabelTestApp(t *testing.T) (*App, *[]relabelCall, *[]string) {
	a, _, _, tabNames := newAgentTestAppWithTab(t)
	calls := &[]relabelCall{}
	a.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript, fallbackTaskID string) {
		*calls = append(*calls, relabelCall{teamID, channelID, threadTS, transcript, fallbackTaskID})
	})
	return a, calls, tabNames
}

func TestLLMLabelRequestedOnceWhenRepliesLoad(t *testing.T) {
	a, calls, tabNames := newLLMLabelTestApp(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562 fix the ingest retries", UserID: "UHUMAN"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "the backoff never resets", UserID: "UBOT"},
		{TS: "102.0", Text: "go with the jittered one", UserID: "UHUMAN"},
	}

	// Every open path paints root-only first: the deterministic label
	// lands, the model request waits for the replies.
	a.setThreadPanel(parent, nil, "C1", "100.0")
	if len(*calls) != 0 {
		t.Fatalf("requested before the replies loaded: %+v", *calls)
	}
	if want := "[colony-562] fix the ingest retries"; len(*tabNames) != 1 || (*tabNames)[0] != want {
		t.Fatalf("tab names = %+v, want the deterministic %q", *tabNames, want)
	}

	a.setThreadPanel(parent, replies, "C1", "100.0")
	if len(*calls) != 1 {
		t.Fatalf("want 1 label request, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.teamID != "T1" || c.channelID != "C1" || c.threadTS != "100.0" {
		t.Errorf("request keyed %+v", c)
	}
	// The same transcript :retitle sends for this panel.
	if want := a.retitleTranscript(parent, replies, "UBOT"); c.transcript != want {
		t.Errorf("transcript = %q, want %q", c.transcript, want)
	}
	for _, want := range []string{"justin: colony-562 fix the ingest retries", "Claude: the backoff never resets", "justin: go with the jittered one"} {
		if !strings.Contains(c.transcript, want) {
			t.Errorf("transcript missing %q:\n%s", want, c.transcript)
		}
	}
	if c.fallbackTaskID != "colony-562" {
		t.Errorf("fallbackTaskID = %q, want the id hoisted from the root", c.fallbackTaskID)
	}

	// The authoritative fetch after a cache prime, or any later reload,
	// must not re-request.
	a.setThreadPanel(parent, replies, "C1", "100.0")
	if len(*calls) != 1 {
		t.Errorf("replies reload re-requested: %+v", *calls)
	}
}

func TestLLMLabelThreadWithoutRepliesSendsRoot(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	a.setThreadPanel(parent, nil, "C1", "100.0")
	// A successful load of a thread nobody replied to yet is empty, not nil.
	a.setThreadPanel(parent, []messages.MessageItem{}, "C1", "100.0")

	if len(*calls) != 1 || (*calls)[0].transcript != "justin: fix the ingest retries" {
		t.Fatalf("want 1 root-only request, got %+v", *calls)
	}
	if got := (*calls)[0].fallbackTaskID; got != "" {
		t.Errorf("fallbackTaskID = %q, want none hoisted", got)
	}
}

func TestLLMLabelPermalinkStubWaitsForBackfilledRoot(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	// A permalink-opened thread starts as a TS-only stub; the replies load
	// backfills the root, which is when the thread is first tracked.
	a.setThreadPanel(messages.MessageItem{TS: "100.0", ThreadTS: "100.0"}, nil, "C1", "100.0")
	if len(*calls) != 0 {
		t.Fatalf("requested off a stub root: %+v", *calls)
	}
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	a.setThreadPanel(parent, []messages.MessageItem{{TS: "101.0", Text: "on it", UserID: "UBOT"}}, "C1", "100.0")
	if len(*calls) != 1 || !strings.Contains((*calls)[0].transcript, "fix the ingest retries") {
		t.Fatalf("want 1 request after backfill, got %+v", *calls)
	}
}

func TestLLMLabelResetOnThreadSwitch(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	none := []messages.MessageItem{}
	a.setThreadPanel(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix retries", UserID: "UHUMAN"}, none, "C1", "100.0")
	a.setThreadPanel(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, none, "C1", "200.0")
	if len(*calls) != 2 || (*calls)[1].threadTS != "200.0" {
		t.Fatalf("want a request per thread, got %+v", *calls)
	}
}

func TestLLMLabelNotRequestedForUntrackedPanelThread(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	// The agent thread is tracked but its replies never loaded; a non-agent
	// thread's replies landing in the panel must not spend its request.
	a.setThreadPanel(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix retries", UserID: "UHUMAN"}, nil, "C1", "100.0")
	a.setThreadPanel(messages.MessageItem{TS: "300.0", Text: "lunch plans", UserID: "UHUMAN"},
		[]messages.MessageItem{{TS: "301.0", Text: "tacos", UserID: "UHUMAN"}}, "C2", "300.0")
	if len(*calls) != 0 {
		t.Fatalf("requested for a thread that is not the tracked one: %+v", *calls)
	}
}

func TestLLMLabelUnconfiguredIsSilent(t *testing.T) {
	a, _, _, tabNames := newAgentTestAppWithTab(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	idle := a.statusbar.View(120)
	a.setThreadPanel(parent, []messages.MessageItem{}, "C1", "100.0")

	if want := "fix the ingest retries"; len(*tabNames) != 1 || (*tabNames)[0] != want {
		t.Errorf("tab names = %+v, want only the deterministic %q", *tabNames, want)
	}
	if out := a.statusbar.View(120); out != idle {
		t.Errorf("the unconfigured automatic request toasted:\nbefore %q\nafter  %q", idle, out)
	}
}

// A failed request answers with nothing (the wiring logs and drops it), so
// silence on failure is the request itself leaving no toast behind and the
// deterministic label staying the last rename.
func TestLLMLabelRequestLeavesNoToast(t *testing.T) {
	a, calls, tabNames := newLLMLabelTestApp(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	idle := a.statusbar.View(120)
	a.setThreadPanel(parent, []messages.MessageItem{}, "C1", "100.0")

	if len(*calls) != 1 {
		t.Fatalf("want 1 label request, got %+v", *calls)
	}
	if out := a.statusbar.View(120); out != idle {
		t.Errorf("statusbar changed by the automatic request:\nbefore %q\nafter  %q", idle, out)
	}
	if want := "fix the ingest retries"; (*tabNames)[len(*tabNames)-1] != want {
		t.Errorf("tab = %q, want the deterministic %q", (*tabNames)[len(*tabNames)-1], want)
	}
}

// A landed result is as quiet as a failed one: the rename is the only
// effect, whether the model found an id or answered none.
func TestLLMLabelResultLeavesNoToast(t *testing.T) {
	a, _, tabNames := newLLMLabelTestApp(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	a.setThreadPanel(parent, []messages.MessageItem{}, "C1", "100.0")
	idle := a.statusbar.View(120)

	cmd, _ := reduceAgentTabRelabel(a, AgentTabRelabelMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Label: "ingest retries"})

	if cmd != nil {
		t.Errorf("reducer returned a cmd; a toast rides on one")
	}
	if out := a.statusbar.View(120); out != idle {
		t.Errorf("statusbar changed by the result:\nbefore %q\nafter  %q", idle, out)
	}
	if last := (*tabNames)[len(*tabNames)-1]; last != "ingest retries" {
		t.Errorf("tab = %q", last)
	}
}

func TestLLMLabelStaleResultDropped(t *testing.T) {
	a, calls, tabNames := newLLMLabelTestApp(t)
	none := []messages.MessageItem{}
	a.setThreadPanel(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix retries", UserID: "UHUMAN"}, none, "C1", "100.0")
	a.setThreadPanel(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, none, "C1", "200.0")
	before := len(*tabNames)

	// The first thread's reply lands after the switch.
	first := (*calls)[0]
	reduceAgentTabRelabel(a, AgentTabRelabelMsg{TeamID: first.teamID, ChannelID: first.channelID, ThreadTS: first.threadTS, Label: "ingest retries"})
	if len(*tabNames) != before {
		t.Errorf("stale result renamed the tab: %+v", *tabNames)
	}
}

func TestSanitizeModelLabel(t *testing.T) {
	cases := []struct{ in, taskID, want string }{
		{"fix ingest retries", "", "fix ingest retries"},
		{"  \"Fix ingest retries\"  ", "", "Fix ingest retries"},
		{"fix retries\nsecond line", "", "fix retries"},
		{"colony-562: fix the flow viewer", "colony-562", "fix the flow viewer"},
		{"a very long label that keeps going well past the cap", "", "a very long label that keeps …"},
		{"\"colony-562\"", "colony-562", ""},
		{"   ", "", ""},
		// Only the id hoisted at request time is stripped: hyphen-digit
		// technical terms the model wrote survive.
		{"fix utf-8 truncation", "", "fix utf-8 truncation"},
		{"migrate hashing to sha-256", "colony-562", "migrate hashing to sha-256"},
		{"PR #1164 verify fix", "#1164", "PR verify fix"},
	}
	for _, c := range cases {
		if got := sanitizeModelLabel(c.in, c.taskID); got != c.want {
			t.Errorf("sanitizeModelLabel(%q, %q) = %q, want %q", c.in, c.taskID, got, c.want)
		}
	}
}
