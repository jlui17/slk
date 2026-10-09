package ui

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

// fakeAgentMarks is an in-memory AgentThreadMarkStore keyed like the cache
// table, shared across Apps to stand in for the DB across a restart.
type fakeAgentMarks map[[3]string]bool

func (f fakeAgentMarks) AgentThreadMarked(ws, ch, ts string) (bool, error) {
	return f[[3]string{ws, ch, ts}], nil
}

func (f fakeAgentMarks) MarkAgentThread(ws, ch, ts string) error {
	f[[3]string{ws, ch, ts}] = true
	return nil
}

func (f fakeAgentMarks) UnmarkAgentThread(ws, ch, ts string) error {
	delete(f, [3]string{ws, ch, ts})
	return nil
}

// A thread whose root names no bot; Claude is first mentioned in a reply.
var (
	markParent  = messages.MessageItem{TS: "100.0", Text: "the ingest retries keep failing", UserID: "UHUMAN"}
	markReplies = []messages.MessageItem{
		{TS: "101.0", Text: "same here since Tuesday", UserID: "UHUMAN"},
		{TS: "102.0", Text: "<@UBOT> can you dig into this", UserID: "UHUMAN"},
		{TS: "103.0", Text: "looking now", UserID: "UBOT"},
	}
)

func TestAgentMarkTracksBotFirstMentionedInReply(t *testing.T) {
	a, calls, _, tabNames := newAgentTestAppWithTab(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	a.setThreadPanel(markParent, markReplies, "C1", "100.0")
	if len(*calls) != 0 || a.agentSidebar.thread.active {
		t.Fatalf("detection tracked a root with no bot: %+v", *calls)
	}

	_ = executeCommand(a, "agent")

	if !marks[[3]string{"T1", "C1", "100.0"}] {
		t.Errorf("mark not saved: %+v", marks)
	}
	th := a.agentSidebar.thread
	if !a.tracksThread("", "C1", "100.0") || th.botUserID != "UBOT" || th.agentName != "Claude" {
		t.Fatalf("tracked %+v, want C1/100.0 with UBOT", th)
	}
	if len(*calls) != 1 || (*calls)[0].agent != "slack-claude" {
		t.Errorf("reports = %+v, want one for slack-claude", *calls)
	}
	if want := agentTabLabel(markParent.Text); len(*tabNames) != 1 || (*tabNames)[0] != want {
		t.Errorf("tab names = %+v, want the root's %q", *tabNames, want)
	}
	if a.agentSidebar.lastMsg.ts != "103.0" {
		t.Errorf("snapshot lastMsg = %+v, want the newest reply", a.agentSidebar.lastMsg)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Marked agent thread") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestAgentMarkWithoutBotToasts(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	a.setThreadPanel(markParent, markReplies[:1], "C1", "100.0")

	_ = executeCommand(a, "agent")

	if len(marks) != 0 || len(*calls) != 0 || a.agentSidebar.thread.active {
		t.Errorf("bot-less thread marked or tracked: marks %+v, reports %+v", marks, *calls)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "No bot in this thread") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestAgentMarkedThreadTrackedOnReopen(t *testing.T) {
	marks := fakeAgentMarks{{"T1", "C1", "100.0"}: true}
	// A fresh App, as after a restart: the mark is only in the store.
	a, calls, _ := newLLMLabelTestApp(t)
	a.SetAgentThreadMarks(marks)

	// Pane restore paints root-only first; the bot is only in a reply,
	// so tracking waits for the replies load.
	a.setThreadPanel(markParent, nil, "C1", "100.0")
	if a.agentSidebar.thread.active {
		t.Fatalf("tracked before any reply named a bot: %+v", a.agentSidebar.thread)
	}
	a.setThreadPanel(markParent, markReplies, "C1", "100.0")

	if !a.tracksThread("", "C1", "100.0") || a.agentSidebar.thread.botUserID != "UBOT" {
		t.Fatalf("marked thread not tracked on reopen: %+v", a.agentSidebar.thread)
	}
	if len(*calls) != 1 {
		t.Errorf("want the automatic label request, got %+v", *calls)
	}
}

func TestAgentUnmarkStopsTrackingOnReopen(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	a.setThreadPanel(markParent, markReplies, "C1", "100.0")
	_ = executeCommand(a, "agent")

	_ = executeCommand(a, "agent")

	if len(marks) != 0 {
		t.Errorf("mark not removed: %+v", marks)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Unmarked agent thread") {
		t.Errorf("statusbar = %q", out)
	}
	// Tracking stays until another agent thread replaces it.
	if !a.tracksThread("", "C1", "100.0") {
		t.Errorf("unmark ended tracking: %+v", a.agentSidebar.thread)
	}

	b, calls, _ := newAgentTestApp(t)
	b.SetAgentThreadMarks(marks)
	b.setThreadPanel(markParent, markReplies, "C1", "100.0")
	if b.agentSidebar.thread.active || len(*calls) != 0 {
		t.Errorf("unmarked thread tracked on reopen: %+v", *calls)
	}
}

func TestAgentMarkOnDetectedThreadKeepsState(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> please fix the ingest retries", UserID: "UHUMAN"}
	a.setThreadPanel(parent, nil, "C1", "100.0")
	a.agentSidebar.addUnread("101.0")
	before := len(*calls)

	_ = executeCommand(a, "agent")

	if !marks[[3]string{"T1", "C1", "100.0"}] {
		t.Errorf("mark not saved: %+v", marks)
	}
	if len(*calls) != before || a.agentSidebar.unreadTotal() != 1 {
		t.Errorf("marking a tracked thread reset it: reports %+v, unread %d", (*calls)[before:], a.agentSidebar.unreadTotal())
	}
}

func TestRetitleAfterAgentMark(t *testing.T) {
	a, calls, _ := newLLMLabelTestApp(t)
	a.SetAgentThreadMarks(fakeAgentMarks{})
	a.setThreadPanel(markParent, markReplies, "C1", "100.0")

	_ = executeCommand(a, "agent")
	if len(*calls) != 1 || (*calls)[0].force {
		t.Fatalf("want the automatic label request on :agent, got %+v", *calls)
	}
	_ = executeCommand(a, "retitle")

	if len(*calls) != 2 || !(*calls)[1].force {
		t.Errorf("want a forced :retitle request, got %+v", *calls)
	}
}

func TestAgentMarkNoThreadToasts(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	a.SetAgentThreadMarks(fakeAgentMarks{})

	_ = executeCommand(a, "agent")

	if out := a.statusbar.View(120); !strings.Contains(out, "Open a thread first") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestAgentMarkOutsideHerdrToasts(t *testing.T) {
	a := newHarnessApp(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	a.threadPanel.SetThread(markParent, markReplies, "C1", "100.0")

	_ = executeCommand(a, "agent")

	if len(marks) != 0 {
		t.Errorf("marked outside herdr: %+v", marks)
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "herdr") {
		t.Errorf("statusbar = %q", out)
	}
}

func TestAgentMarkSkipsEphemeralBot(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	marks := fakeAgentMarks{}
	a.SetAgentThreadMarks(marks)
	ephemeral := messages.MessageItem{TS: "101.5", Text: "only visible to you", UserID: "UBOT", IsEphemeral: true}
	a.setThreadPanel(markParent, []messages.MessageItem{markReplies[0], ephemeral}, "C1", "100.0")

	_ = executeCommand(a, "agent")

	if len(marks) != 0 || len(*calls) != 0 || a.agentSidebar.thread.active {
		t.Errorf("thread marked or tracked from an ephemeral: marks %+v, reports %+v", marks, *calls)
	}
}

func TestAgentMarkedThreadReloadKeepsState(t *testing.T) {
	a, calls, _ := newAgentTestApp(t)
	a.SetAgentThreadMarks(fakeAgentMarks{})
	a.setThreadPanel(markParent, markReplies, "C1", "100.0")
	_ = executeCommand(a, "agent")
	a.agentSidebar.addUnread("103.0")
	reports := len(*calls)

	a.setThreadPanel(markParent, markReplies, "C1", "100.0")

	if got := a.agentSidebar.unreadTotal(); got != 1 {
		t.Errorf("unread after reload = %d, want 1", got)
	}
	if len(*calls) != reports {
		t.Errorf("reload re-reported the thread: %+v", (*calls)[reports:])
	}
}
