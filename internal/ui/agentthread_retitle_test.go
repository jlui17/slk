package ui

import (
	"fmt"
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
	n := maxRetitleTranscript/maxRetitleReply + 50
	for i := 0; i < n; i++ {
		replies = append(replies, messages.MessageItem{
			TS: "101.0", Text: fmt.Sprintf("reply%04d %s", i, big), UserID: "UHUMAN",
		})
	}
	a, calls, _ := newRetitleTestApp(t, parent, replies)

	_ = executeCommand(a, "retitle")

	c := (*calls)[0]
	if len(c.transcript) > maxRetitleTranscript {
		t.Errorf("transcript over budget: %d bytes", len(c.transcript))
	}
	if !strings.Contains(c.transcript, fmt.Sprintf("reply%04d", n-1)) {
		t.Errorf("newest reply dropped:\n%.200s", c.transcript)
	}
	if strings.Contains(c.transcript, "reply0000 ") {
		t.Errorf("oldest reply kept despite budget")
	}
	if !strings.Contains(c.transcript, "do the thing") {
		t.Errorf("root dropped")
	}
}

func TestRelabelResultAppliesModelTaskID(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)

	if _, handled := reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "#1170", Label: "#1170 CI workflow optimization",
	}); !handled {
		t.Fatal("relabel msg not handled")
	}
	last := (*tabNames)[len(*tabNames)-1]
	if last != "[#1170] CI workflow optimization" {
		t.Errorf("tab = %q, want the model id hoisted and its echo stripped", last)
	}
	if got := a.agentSidebar.llmLabel.taskID; got != "#1170" {
		t.Errorf("taskID = %q", got)
	}
}

func TestRelabelResultNoIDClearsStaleID(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)
	a.agentSidebar.llmLabel.taskID = "issuecomment-15686"

	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "", Label: "CI workflow optimization",
	})

	last := (*tabNames)[len(*tabNames)-1]
	if last != "CI workflow optimization" {
		t.Errorf("tab = %q, want no id prefix", last)
	}
	if got := a.agentSidebar.llmLabel.taskID; got != "" {
		t.Errorf("taskID = %q, want the stale id dropped", got)
	}
}

func TestRelabelResultThreadMismatchDropped(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)
	before := len(*tabNames)

	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "999.0", TaskID: "#1170", Label: "Stale result",
	})

	if len(*tabNames) != before {
		t.Errorf("stale result renamed the tab: %+v", *tabNames)
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
