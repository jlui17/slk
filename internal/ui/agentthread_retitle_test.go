package ui

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

type relabelCall struct {
	teamID     string
	channelID  string
	threadTS   string
	transcript string
}

// newRetitleTestApp tracks an agent thread whose panel holds replies, with
// both label generators installed, and returns the relabel capture and the
// NameTab capture.
func newRetitleTestApp(t *testing.T, parent messages.MessageItem, replies []messages.MessageItem) (*App, *[]relabelCall, *[]string) {
	t.Helper()
	a, _, tabNames := newLLMLabelTestApp(t)
	calls := &[]relabelCall{}
	a.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript string) {
		*calls = append(*calls, relabelCall{teamID, channelID, threadTS, transcript})
	})
	a.threadPanel.SetThread(parent, replies, "C1", parent.TS)
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", parent.TS)
	return a, calls, tabNames
}

func TestRetitleRequestsRecentTranscript(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> brainstorm the retry design", UserID: "UHUMAN"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "sketching two options", UserID: "UBOT"},
		{TS: "102.0", Text: "implementing option two now", UserID: "UHUMAN"},
	}
	a, calls, _ := newRetitleTestApp(t, parent, replies)

	_ = executeCommand(a, "retitle")

	if len(*calls) != 1 {
		t.Fatalf("want 1 relabel request, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.teamID != "T1" || c.channelID != "C1" || c.threadTS != "100.0" {
		t.Errorf("request keyed %+v", c)
	}
	root := strings.Index(c.transcript, "brainstorm the retry design")
	first := strings.Index(c.transcript, "Claude: sketching two options")
	second := strings.Index(c.transcript, "justin: implementing option two now")
	if root < 0 || first < 0 || second < 0 {
		t.Fatalf("transcript missing messages:\n%s", c.transcript)
	}
	if !(root < first && first < second) {
		t.Errorf("transcript not chronological:\n%s", c.transcript)
	}
	if strings.Contains(c.transcript, "<@UBOT>") {
		t.Errorf("bot mention survived in root line:\n%s", c.transcript)
	}
}

func TestRetitleBudgetKeepsNewestReplies(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> do the thing", UserID: "UHUMAN"}
	big := strings.Repeat("x", maxRetitleReply-100)
	var replies []messages.MessageItem
	for i := 0; i < 12; i++ {
		replies = append(replies, messages.MessageItem{
			TS: "101.0", Text: "reply" + string(rune('a'+i)) + " " + big, UserID: "UHUMAN",
		})
	}
	a, calls, _ := newRetitleTestApp(t, parent, replies)

	_ = executeCommand(a, "retitle")

	c := (*calls)[0]
	if len(c.transcript) > maxRetitleTranscript {
		t.Errorf("transcript over budget: %d bytes", len(c.transcript))
	}
	if !strings.Contains(c.transcript, "replyl") {
		t.Errorf("newest reply dropped:\n%.200s", c.transcript)
	}
	if strings.Contains(c.transcript, "replya ") {
		t.Errorf("oldest reply kept despite budget")
	}
	if !strings.Contains(c.transcript, "do the thing") {
		t.Errorf("root dropped")
	}
}

func TestRetitleTaskIDNewestMentionWins(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562 fix the viewer", UserID: "UHUMAN"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "filed as colony-999, starting", UserID: "UBOT"},
	}
	a, _, tabNames := newRetitleTestApp(t, parent, replies)

	_ = executeCommand(a, "retitle")

	if got := a.agentSidebar.llmLabel.taskID; got != "colony-999" {
		t.Fatalf("taskID = %q, want the reply's colony-999", got)
	}
	if _, handled := reduceAgentTabLabel(a, AgentTabLabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Label: "Implement viewer fix",
	}); !handled {
		t.Fatal("label msg not handled")
	}
	last := (*tabNames)[len(*tabNames)-1]
	if last != "[colony-999] Implement viewer fix" {
		t.Errorf("tab = %q", last)
	}
}

func TestRetitleKeepsTaskIDWhenRecentContextHasNone(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562 fix the viewer", UserID: "UHUMAN"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "done with the first pass", UserID: "UBOT"},
	}
	a, _, _ := newRetitleTestApp(t, parent, replies)
	// Simulate a hoist that happened before the replies existed, with a
	// root the panel no longer carries the id from.
	a.agentSidebar.llmLabel = llmLabelState{requested: true, taskID: "colony-777"}
	a.threadPanel.SetThread(messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}, replies, "C1", "100.0")

	_ = executeCommand(a, "retitle")

	if got := a.agentSidebar.llmLabel.taskID; got != "colony-777" {
		t.Errorf("taskID = %q, want the previously hoisted colony-777 kept", got)
	}
}

func TestRetitleNoTrackedThreadToasts(t *testing.T) {
	a, _, _ := newLLMLabelTestApp(t)
	calls := &[]relabelCall{}
	a.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript string) {
		*calls = append(*calls, relabelCall{teamID, channelID, threadTS, transcript})
	})

	_ = executeCommand(a, "retitle")

	if len(*calls) != 0 {
		t.Fatalf("requested with no tracked thread: %+v", *calls)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "No agent thread tracked") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestRetitlePanelOnDifferentThreadToasts(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, calls, _ := newRetitleTestApp(t, parent, nil)
	a.threadPanel.SetThread(messages.MessageItem{TS: "300.0", Text: "lunch plans", UserID: "UHUMAN"}, nil, "C2", "300.0")

	_ = executeCommand(a, "retitle")

	if len(*calls) != 0 {
		t.Fatalf("requested off a different thread's panel: %+v", *calls)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Open the agent thread first") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestRetitleUnconfiguredToasts(t *testing.T) {
	a, _, _ := newLLMLabelTestApp(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.updateAgentThread(parent, "C1", "100.0")

	_ = executeCommand(a, "retitle")

	if out := a.statusbar.View(120); !strings.Contains(out, "not configured") {
		t.Errorf("statusbar = %q", out)
	}
}
