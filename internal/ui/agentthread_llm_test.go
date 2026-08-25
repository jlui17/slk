package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

type labelGenCall struct {
	teamID    string
	channelID string
	threadTS  string
	root      string
}

func newLLMLabelTestApp() (*App, *[]labelGenCall, *[]string) {
	a, _, _, tabNames := newAgentTestAppWithTab()
	gens := &[]labelGenCall{}
	a.SetAgentTabLabeler(func(teamID, channelID, threadTS, root string) {
		*gens = append(*gens, labelGenCall{teamID, channelID, threadTS, root})
	})
	return a, gens, tabNames
}

func TestLLMLabelRequestedOncePerThread(t *testing.T) {
	a, gens, _ := newLLMLabelTestApp()
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the ingest retries", UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", "100.0")

	if len(*gens) != 1 {
		t.Fatalf("want 1 label request, got %+v", *gens)
	}
	g := (*gens)[0]
	if g.teamID != "T1" || g.channelID != "C1" || g.threadTS != "100.0" {
		t.Errorf("request keyed %+v", g)
	}
	if g.root != "fix the ingest retries" {
		t.Errorf("root = %q, want mention-stripped text", g.root)
	}

	// A replies reload through the same-thread path must not re-request.
	a.updateAgentThread(parent, "C1", "100.0")
	if len(*gens) != 1 {
		t.Errorf("same-thread refresh re-requested: %+v", *gens)
	}
}

func TestLLMLabelDeferredUntilRootTextArrives(t *testing.T) {
	a, gens, _ := newLLMLabelTestApp()
	// A permalink-opened thread tracks with an empty root; the text
	// backfills through a same-thread refresh.
	parent := messages.MessageItem{TS: "100.0", Text: "", UserID: "UBOT"}
	a.updateAgentThread(parent, "C1", "100.0")
	if len(*gens) != 0 {
		t.Fatalf("requested with no root text: %+v", *gens)
	}
	parent.Text = "kicking off the retry fix"
	a.updateAgentThread(parent, "C1", "100.0")
	if len(*gens) != 1 || (*gens)[0].root != "kicking off the retry fix" {
		t.Fatalf("want 1 request after backfill, got %+v", *gens)
	}
}

func TestLLMLabelResetOnThreadSwitch(t *testing.T) {
	a, gens, _ := newLLMLabelTestApp()
	a.updateAgentThread(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix retries", UserID: "UHUMAN"}, "C1", "100.0")
	a.updateAgentThread(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, "C1", "200.0")
	if len(*gens) != 2 || (*gens)[1].threadTS != "200.0" {
		t.Fatalf("want a request per thread, got %+v", *gens)
	}
}

func TestLLMLabelResultRenamesTab(t *testing.T) {
	a, _, tabNames := newLLMLabelTestApp()
	a.updateAgentThread(messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562 fix the flow viewer", UserID: "UHUMAN"}, "C1", "100.0")
	before := len(*tabNames)

	if _, handled := reduceAgentTabLabel(a, AgentTabLabelMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Label: "\"Fix flow viewer rendering.\"\nextra"}); !handled {
		t.Fatal("reducer must claim AgentTabLabelMsg")
	}
	if len(*tabNames) != before+1 {
		t.Fatalf("want a rename, got %+v", *tabNames)
	}
	// Sanitized (first line, quotes gone) and re-prefixed with the task id
	// hoisted from the root at request time.
	if want := "[colony-562] Fix flow viewer rendering."; (*tabNames)[len(*tabNames)-1] != want {
		t.Errorf("label = %q, want %q", (*tabNames)[len(*tabNames)-1], want)
	}
}

func TestLLMLabelStaleResultDropped(t *testing.T) {
	a, _, tabNames := newLLMLabelTestApp()
	a.updateAgentThread(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix retries", UserID: "UHUMAN"}, "C1", "100.0")
	a.updateAgentThread(messages.MessageItem{TS: "200.0", Text: "<@UBOT> ship the viewer", UserID: "UHUMAN"}, "C1", "200.0")
	before := len(*tabNames)

	reduceAgentTabLabel(a, AgentTabLabelMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Label: "fix retries"})
	if len(*tabNames) != before {
		t.Errorf("stale result renamed the tab: %+v", *tabNames)
	}
}

func TestLLMLabelUnusableResultDropped(t *testing.T) {
	a, _, tabNames := newLLMLabelTestApp()
	a.updateAgentThread(messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562", UserID: "UHUMAN"}, "C1", "100.0")
	before := len(*tabNames)

	// Nothing left once the echoed task id and quotes are stripped.
	reduceAgentTabLabel(a, AgentTabLabelMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Label: "\"colony-562\""})
	if len(*tabNames) != before {
		t.Errorf("empty-after-sanitize result renamed the tab: %+v", *tabNames)
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
	}
	for _, c := range cases {
		if got := sanitizeModelLabel(c.in, c.taskID); got != c.want {
			t.Errorf("sanitizeModelLabel(%q, %q) = %q, want %q", c.in, c.taskID, got, c.want)
		}
	}
}
