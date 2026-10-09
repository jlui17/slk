package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/usernames"
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
	a.updateAgentThread(parent, nil, "C1", parent.TS)
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

// relabelResult is the result the live wiring answers a request with: the
// request's keys and :retitle number echoed beside the model's label.
func relabelResult(c relabelCall, label string) AgentTabRelabelMsg {
	return AgentTabRelabelMsg{
		TeamID: c.teamID, ChannelID: c.channelID, ThreadTS: c.threadTS,
		FallbackTaskID: c.fallbackTaskID, RetitleGen: c.retitleGen, Label: label,
	}
}

func TestRetitleResultForcesRename(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	replies := []messages.MessageItem{{TS: "101.0", Text: "on it", UserID: "UBOT"}}
	a, calls, tabNames := newRetitleTestApp(t, parent, replies)
	var forced []string
	a.SetAgentTabForceNamer(func(label string) { forced = append(forced, label) })
	before := len(*tabNames)

	_ = executeCommand(a, "retitle")
	_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[0], "Viewer stale runs"))

	if len(forced) != 1 || forced[0] != "Viewer stale runs" {
		t.Errorf("forced renames = %+v, want the :retitle label", forced)
	}
	if len(*tabNames) != before {
		t.Errorf(":retitle result took the guarded rename: %+v", (*tabNames)[before:])
	}
}

func TestOpenTimeResultKeepsGuardedRename(t *testing.T) {
	a, calls, tabNames := newLLMLabelTestApp(t)
	var forced []string
	a.SetAgentTabForceNamer(func(label string) { forced = append(forced, label) })
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	replies := []messages.MessageItem{{TS: "101.0", Text: "on it", UserID: "UBOT"}}

	a.setThreadPanel(parent, nil, "C1", "100.0")
	a.setThreadPanel(parent, replies, "C1", "100.0")
	if len(*calls) != 1 {
		t.Fatalf("want 1 open-time request, got %+v", *calls)
	}
	_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[0], "Viewer stale runs"))

	if len(forced) != 0 {
		t.Errorf("an automatic label forced the rename: %+v", forced)
	}
	if last := (*tabNames)[len(*tabNames)-1]; last != "Viewer stale runs" {
		t.Errorf("tab = %q, want the model label through the guarded rename", last)
	}
}

func TestForcedResultWithoutForceNamerFallsBack(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, calls, tabNames := newRetitleTestApp(t, parent, nil)

	_ = executeCommand(a, "retitle")
	_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[0], "Viewer stale runs"))

	if last := (*tabNames)[len(*tabNames)-1]; last != "Viewer stale runs" {
		t.Errorf("tab = %q, want the guarded rename when no forced one is installed", last)
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

func TestRetitleNoThreadOpenToasts(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)

	_ = executeCommand(a, "retitle")

	if len(*calls) != 0 {
		t.Fatalf("requested with no thread open: %+v", *calls)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Open a thread first") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestRetitleOutsideHerdrToasts(t *testing.T) {
	a := newHarnessApp(t)
	a.threadPanel.SetThread(lunchParent, lunchReplies, "C2", "300.0")

	_ = executeCommand(a, "retitle")

	if out := a.statusbar.View(120); !strings.Contains(out, "herdr") {
		t.Errorf("statusbar = %q", out)
	}
}

// lunchParent and lunchReplies are a thread with no bot in it: never an
// agent thread.
var (
	lunchParent  = messages.MessageItem{TS: "300.0", Text: "lunch plans for the offsite", UserID: "UHUMAN"}
	lunchReplies = []messages.MessageItem{{TS: "301.0", Text: "tacos on thursday", UserID: "USELF"}}
)

// newRetitleOtherThreadApp tracks an agent thread, then shows a thread
// with no bot in the panel, the way opening one leaves the tracked thread
// tracked.
func newRetitleOtherThreadApp(t *testing.T) (*App, *[]relabelCall, *[]string, *[]string) {
	t.Helper()
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, calls, tabNames := newRetitleTestApp(t, parent, nil)
	forced := &[]string{}
	a.SetAgentTabForceNamer(func(label string) { *forced = append(*forced, label) })
	a.setThreadPanel(lunchParent, lunchReplies, "C2", "300.0")
	if !a.tracksThread("", "C1", "100.0") {
		t.Fatal("opening a thread with no bot replaced the tracked thread")
	}
	return a, calls, tabNames, forced
}

func TestRetitleOnOtherThreadNamesTabOnly(t *testing.T) {
	a, calls, tabNames, forced := newRetitleOtherThreadApp(t)
	tracked := a.agentSidebar.thread
	namesBefore := len(*tabNames)

	_ = executeCommand(a, "retitle")

	if len(*calls) != 1 {
		t.Fatalf("want 1 relabel request, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.teamID != "T1" || c.channelID != "C2" || c.threadTS != "300.0" || !c.force || c.fallbackTaskID != "" {
		t.Errorf("request = %+v, want a forced request keyed to the open thread", c)
	}
	if want := "justin: lunch plans for the offsite\ntacos on thursday"; c.transcript != want {
		t.Errorf("transcript = %q, want %q", c.transcript, want)
	}

	_, _ = reduceAgentTabRelabel(a, relabelResult(c, "offsite lunch"))

	if len(*forced) != 1 || (*forced)[0] != "offsite lunch" {
		t.Errorf("forced renames = %+v, want the :retitle label", *forced)
	}
	if len(*tabNames) != namesBefore {
		t.Errorf("guarded rename ran: %+v", (*tabNames)[namesBefore:])
	}
	if a.agentSidebar.thread != tracked {
		t.Errorf("tracked thread changed: %+v, was %+v", a.agentSidebar.thread, tracked)
	}
}

// The user asked for the tab to be named after the thread: the answer
// lands wherever the panel has gone since, :retitle then esc included.
func TestRetitleResultLandsAfterThreadLeft(t *testing.T) {
	agentParent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	open := map[string]func(t *testing.T) (*App, *[]relabelCall){
		"agent thread": func(t *testing.T) (*App, *[]relabelCall) {
			a, calls, _ := newRetitleTestApp(t, agentParent, nil)
			return a, calls
		},
		"other thread": func(t *testing.T) (*App, *[]relabelCall) {
			a, calls, _, _ := newRetitleOtherThreadApp(t)
			return a, calls
		},
	}
	leave := map[string]func(a *App){
		"esc": func(a *App) { a.CloseThread() },
		"another thread opened": func(a *App) {
			a.setThreadPanel(messages.MessageItem{TS: "400.0", Text: "standup notes", UserID: "UHUMAN"}, nil, "C2", "400.0")
		},
	}
	for openName, openThread := range open {
		for leaveName, leaveThread := range leave {
			t.Run(openName+"/"+leaveName, func(t *testing.T) {
				a, calls := openThread(t)
				var forced []string
				a.SetAgentTabForceNamer(func(label string) { forced = append(forced, label) })

				_ = executeCommand(a, "retitle")
				leaveThread(a)
				_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[len(*calls)-1], "named by retitle"))

				if len(forced) != 1 || forced[0] != "named by retitle" {
					t.Errorf("forced renames = %+v, want the :retitle label", forced)
				}
			})
		}
	}
}

// The latest :retitle wins: an older answer arriving after a newer
// :retitle on another thread was requested is dropped.
func TestRetitleOlderResultDroppedAfterNewerRetitle(t *testing.T) {
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a, calls, _ := newRetitleTestApp(t, parent, nil)
	var forced []string
	a.SetAgentTabForceNamer(func(label string) { forced = append(forced, label) })

	_ = executeCommand(a, "retitle")
	a.setThreadPanel(lunchParent, lunchReplies, "C2", "300.0")
	_ = executeCommand(a, "retitle")
	_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[1], "offsite lunch"))
	_, _ = reduceAgentTabRelabel(a, relabelResult((*calls)[0], "viewer fix"))

	if len(forced) != 1 || forced[0] != "offsite lunch" {
		t.Errorf("forced renames = %+v, want only the newer :retitle's label", forced)
	}
}

// A :retitle waiting on the annotated message's fetch is superseded by a
// newer :retitle like any other: the fetch's request never goes out.
func TestReviewOpenRetitleFetchDroppedAfterNewerRetitle(t *testing.T) {
	a, calls, _, _ := newRetitleOtherThreadApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	a.threadPanel.SetThread(parent, replies, "C3", "500.0")
	fetch := executeCommand(a, "retitle")
	a.threadPanel.SetThread(lunchParent, lunchReplies, "C2", "300.0")
	_ = executeCommand(a, "retitle")

	_, _ = reduceAgentTabRelabel(a, fetch().(tea.BatchMsg)[0]())

	if len(*calls) != 1 || (*calls)[0].threadTS != "300.0" {
		t.Fatalf("want only the newer :retitle's request, got %+v", *calls)
	}
}

// A !review-open thread that isn't tracked still fetches its annotated
// message, and the request goes out keyed to the open thread.
func TestReviewOpenRetitleOnOtherThread(t *testing.T) {
	a, calls, _, _ := newRetitleOtherThreadApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	a.threadPanel.SetThread(parent, replies, "C3", "500.0")

	cmd := executeCommand(a, "retitle")
	_, _ = reduceAgentTabRelabel(a, cmd().(tea.BatchMsg)[0]())

	if len(*calls) != 1 {
		t.Fatalf("want 1 label request, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.channelID != "C3" || c.threadTS != "500.0" || !c.force || !c.reviewOpen {
		t.Errorf("request = %+v, want a forced review-open request keyed to the open thread", c)
	}
	if !strings.Contains(c.transcript, "\nJustin Lui: pr 1231 is merged already\n") {
		t.Errorf("transcript:\n%s", c.transcript)
	}
}

func TestRetitleUnconfiguredToasts(t *testing.T) {
	a, _, _, _ := newAgentTestAppWithTab(t)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> fix the viewer", UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.updateAgentThread(parent, nil, "C1", "100.0")

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

func TestRetitleTranscriptDropsSpeakerSessionLabel(t *testing.T) {
	a, _, _ := newLLMLabelTestApp(t)
	a.SetUserNames(usernames.FromMap(map[string]string{"B1": "Claude [reviewing bootspec PR]"}))
	parent := messages.MessageItem{TS: "100.0", Text: "Blind judge, Justin 5", UserID: "B1", UserName: "Claude [judging Justin annotation 5]"}
	replies := []messages.MessageItem{
		{TS: "101.0", Text: "Blind judge run started", UserID: "B1", UserName: "Claude [judging Justin annotation 5]"},
	}

	got := a.retitleTranscript(parent, replies, "UBOT", "")

	if want := "Claude: Blind judge, Justin 5\nClaude: Blind judge run started"; got != want {
		t.Errorf("transcript =\n%s\nwant\n%s", got, want)
	}
	if strings.Contains(got, "bootspec") {
		t.Errorf("transcript carries the cached session label:\n%s", got)
	}
}

func TestStripSessionLabel(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain name unchanged", "Claude", "Claude"},
		{"trailing label stripped", "Claude [reviewing bootspec PR]", "Claude"},
		{"label alone unchanged", "[x]", "[x]"},
		{"bracket not at the end unchanged", "Claude [beta] bot", "Claude [beta] bot"},
	}
	for _, c := range cases {
		if got := stripSessionLabel(c.in); got != c.want {
			t.Errorf("%s: stripSessionLabel(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

func TestRetitleOnOtherThreadKeepsBotMention(t *testing.T) {
	a, calls, _, _ := newRetitleOtherThreadApp(t)
	a.threadPanel.SetThread(messages.MessageItem{TS: "400.0", Text: "<@UBOT> lunch plans", UserID: "UHUMAN"}, nil, "C4", "400.0")

	_ = executeCommand(a, "retitle")

	if len(*calls) != 1 || !strings.Contains((*calls)[0].transcript, "@Claude lunch plans") {
		t.Errorf("want the untracked thread's bot mention kept, got %+v", *calls)
	}
}

func TestReviewOpenRetitleFetchSurvivesAutomaticFetch(t *testing.T) {
	a, calls, _, _ := newRetitleOtherThreadApp(t)
	withPreview(a, "Justin Lui", "!annotate - pr 1231 is merged already", nil)
	parent, replies := reviewOpenThread()
	a.threadPanel.SetThread(parent, replies, "C3", "500.0")

	cmd := executeCommand(a, "retitle")
	// An agent thread opening before the fetch answers starts its own.
	a.agentSidebar.labelFetchGen++
	_, _ = reduceAgentTabRelabel(a, cmd().(tea.BatchMsg)[0]())

	if len(*calls) != 1 || (*calls)[0].threadTS != "500.0" {
		t.Errorf("want the :retitle request sent, got %+v", *calls)
	}
}
