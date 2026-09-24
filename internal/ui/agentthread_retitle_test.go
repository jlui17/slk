package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

// newRetitleTestApp tracks an agent thread whose panel holds replies, with
// the label generator installed, and returns the relabel capture and the
// NameTab capture. It tracks without setThreadPanel, so the automatic
// open-time request stays out of the capture.
func newRetitleTestApp(t *testing.T, parent messages.MessageItem, replies []messages.MessageItem) (*App, *[]relabelCall, *[]string) {
	t.Helper()
	a, calls, tabNames := newLLMLabelTestApp(t)
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
	if c.fallbackTaskID != "" {
		t.Errorf("fallbackTaskID = %q: on :retitle the model's none is authoritative", c.fallbackTaskID)
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
}

func TestRelabelResultNoIDClearsStaleID(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> sha-256 the viewer cache keys", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)
	if first := (*tabNames)[0]; !strings.HasPrefix(first, "[sha-256]") {
		t.Fatalf("deterministic label = %q, want the wrongly hoisted id this test drops", first)
	}

	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "", Label: "viewer cache keys",
	})

	last := (*tabNames)[len(*tabNames)-1]
	if last != "viewer cache keys" {
		t.Errorf("tab = %q, want no id prefix", last)
	}
}

func TestRelabelResultNoIDKeepsFallbackID(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> please implement colony-1620", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)

	// The open-time request carries the root's hoisted id; a model none
	// must not strip it off the tab.
	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "", FallbackTaskID: "colony-1620", Label: "colony-1620 twin rewind",
	})

	last := (*tabNames)[len(*tabNames)-1]
	if last != "[colony-1620] twin rewind" {
		t.Errorf("tab = %q, want the fallback id kept and its echo stripped", last)
	}
}

func TestRelabelResultModelIDBeatsFallbackID(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> Sim env issue 23: sha-256 mismatch on timeout", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)

	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "sim23", FallbackTaskID: "sha-256", Label: "shell timeout",
	})

	last := (*tabNames)[len(*tabNames)-1]
	if last != "[sim23] shell timeout" {
		t.Errorf("tab = %q, want the model's id over the hoisted one", last)
	}
}

func TestRelabelResultUnusableLabelDropped(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> colony-562 fix the flow viewer", UserID: "UHUMAN"}
	a, _, tabNames := newRetitleTestApp(t, parent, nil)
	before := len(*tabNames)

	// Nothing left once the echoed id and quotes are stripped, and no id of
	// the model's own: the deterministic label must stand, not shrink to
	// the bare fallback id.
	_, _ = reduceAgentTabRelabel(a, AgentTabRelabelMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: "", FallbackTaskID: "colony-562", Label: "\"colony-562\"",
	})

	if len(*tabNames) != before {
		t.Errorf("empty-after-sanitize result renamed the tab: %+v", *tabNames)
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
	a, calls, _ := newLLMLabelTestApp(t)

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
	a, _, _, _ := newAgentTestAppWithTab(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.updateAgentThread(parent, "C1", "100.0")

	_ = executeCommand(a, "retitle")

	if out := a.statusbar.View(120); !strings.Contains(out, "not configured") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestStripLinkTargets(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"markdown link keeps its label", "see [the triage thread](https://acme-x1658.slack.com/archives/C1/p1790) for why", "see the triage thread for why"},
		{"markdown link labelled with a PR ref", "merged [#2079](https://git.example.com/acme/app/pulls/2079) now", "merged #2079 now"},
		{"bare url is dropped", "root cause of issue 3 in https://docs.example.com/d/1bpw/edit?tab=t.z0 please", "root cause of issue 3 in please"},
		{"bare url at the end", "the doc: https://docs.example.com/d/1bpw", "the doc:"},
		{"two links in one line", "[a](https://x.example/1) and [b](http://y.example/2)", "a and b"},
		{"brackets without a target stay", "Claude [reviewing a PR]: todo (see notes)", "Claude [reviewing a PR]: todo (see notes)"},
		{"no link", "PROJ-123 viewer fix", "PROJ-123 viewer fix"},
	}
	for _, c := range cases {
		if got := stripLinkTargets(c.in); got != c.want {
			t.Errorf("%s: stripLinkTargets(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRetitleTranscriptDropsLinkTargets(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer, context in <https://acme-x1658.slack.com/archives/C1/p1790>", UserID: "UHUMAN"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "opened [#1170](https://git.example.com/acme/app/pulls/1170)", UserID: "UBOT"},
	}
	a, calls, _ := newRetitleTestApp(t, parent, replies)

	_ = executeCommand(a, "retitle")

	if len(*calls) != 1 {
		t.Fatalf("want 1 relabel request, got %+v", *calls)
	}
	transcript := (*calls)[0].transcript
	if strings.Contains(transcript, "http") || strings.Contains(transcript, "acme-x1658") {
		t.Errorf("transcript still carries a link target:\n%s", transcript)
	}
	if !strings.Contains(transcript, "opened #1170") {
		t.Errorf("transcript lost the link's label:\n%s", transcript)
	}
}
