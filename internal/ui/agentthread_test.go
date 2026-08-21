package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
	"github.com/gammons/slk/internal/usernames"
)

type agentReportCall struct {
	agent       string
	displayName string
	title       string
	working     bool
	status      string
}

type agentUnreadCall struct {
	agent       string
	displayName string
	title       string
	status      string
}

// newAgentTestApp wires an App with recording agent callbacks and a fixed
// user cache: UBOT is a bot ("Claude"), UHUMAN is not, everything else is
// uncached.
func newAgentTestApp() (*App, *[]agentReportCall, *[]agentUnreadCall) {
	a, calls, unreads, _ := newAgentTestAppWithTab()
	return a, calls, unreads
}

func newAgentTestAppWithTab() (*App, *[]agentReportCall, *[]agentUnreadCall, *[]string) {
	a := NewApp()
	calls := &[]agentReportCall{}
	unreads := &[]agentUnreadCall{}
	tabNames := &[]string{}
	a.SetAgentReporter(
		func(agent, displayName, title string, working bool, statusMessage string) {
			*calls = append(*calls, agentReportCall{agent, displayName, title, working, statusMessage})
		},
		func(agent, displayName, title, statusMessage string) {
			*unreads = append(*unreads, agentUnreadCall{agent, displayName, title, statusMessage})
		},
		func(label string) { *tabNames = append(*tabNames, label) },
		func(userID string) (string, bool, bool) {
			switch userID {
			case "UBOT":
				return "Claude", true, true
			case "UHUMAN":
				return "justin", false, true
			}
			return "", false, false
		},
	)
	a.channelNames = map[string]string{"C1": "z-claude-dreams"}
	a.currentUserID = "USELF"
	// A real workspace id, so tracking captures one and the workspace
	// comparison every hook makes is actually exercised rather than
	// passing on two empty strings.
	a.activeTeamID = "T1"
	return a, calls, unreads, tabNames
}

func openAgentThread(a *App, text string) {
	parent := messages.MessageItem{TS: "100.0", Text: text, UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", "100.0")
}

func TestAgentThreadDetectedFromBotMention(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> please fix the ingest retries")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.agent != "slack-claude" || c.displayName != "Claude" || c.working {
		t.Errorf("unexpected report: %+v", c)
	}
	if want := "#z-claude-dreams @Claude please fix the ingest retries"; c.title != want {
		t.Errorf("title = %q, want %q", c.title, want)
	}
}

func TestAgentThreadDetectedFromBotAuthor(t *testing.T) {
	a, calls, _, tabNames := newAgentTestAppWithTab()
	parent := messages.MessageItem{TS: "100.0", Text: "kicking off the ingest retry fix", UserID: "UBOT"}
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(parent, "C1", "100.0")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	c := (*calls)[0]
	if c.agent != "slack-claude" || c.displayName != "Claude" || c.working {
		t.Errorf("unexpected report: %+v", c)
	}
	if want := "#z-claude-dreams kicking off the ingest retry fix"; c.title != want {
		t.Errorf("title = %q, want %q", c.title, want)
	}
	if len(*tabNames) != 1 || (*tabNames)[0] != "kicking off the ingest retry …" {
		t.Errorf("tab names = %+v", *tabNames)
	}
}

func TestAgentThreadNamesTab(t *testing.T) {
	a, _, _, tabNames := newAgentTestAppWithTab()
	openAgentThread(a, "<@UBOT> please fix the ingest retries in colony")

	if len(*tabNames) != 1 {
		t.Fatalf("want 1 tab name, got %+v", *tabNames)
	}
	// The leading agent mention is dropped and the label truncated to 30
	// cells; closing the thread does not rename back.
	if want := "please fix the ingest retries…"; (*tabNames)[0] != want {
		t.Errorf("tab name = %q, want %q", (*tabNames)[0], want)
	}
	a.CloseThread()
	if len(*tabNames) != 1 {
		t.Errorf("close must not rename the tab; got %+v", *tabNames)
	}
}

func TestAgentTabLabelHoistsTaskID(t *testing.T) {
	cases := []struct{ flat, want string }{
		{"colony-562: fix the flow viewer", "[colony-562] fix the flow viewer"},
		{"please babysit colony-71 until merge", "[colony-71] please babysit until merge"},
		{"colony-562", "[colony-562]"},
		{"fix the ingest retries", "fix the ingest retries"},
		// Lifting the id out of mid-sentence must not strand the
		// punctuation that surrounded it.
		{"verification traffic for slk-373, the whole path", "[slk-373] verification traffic for the …"},
		{"see colony-71 - then ship it", "[colony-71] see then ship it"},
		{"ship colony-9.", "[colony-9] ship"},
	}
	for _, c := range cases {
		if got := agentTabLabel(c.flat); got != c.want {
			t.Errorf("agentTabLabel(%q) = %q, want %q", c.flat, got, c.want)
		}
	}
}

func TestAgentTabNameIndependentOfNameSources(t *testing.T) {
	a, _, _, tabNames := newAgentTestAppWithTab()
	// The in-memory name map and the user cache disagree on the bot's
	// name; the tab label must not carry a mangled fragment of either.
	a.userNames = usernames.FromMap(map[string]string{"UBOT": "Claude Tag"})
	openAgentThread(a, "<@UBOT> fix the retries")

	if len(*tabNames) != 1 || (*tabNames)[0] != "fix the retries" {
		t.Errorf("tab names = %+v, want [fix the retries]", *tabNames)
	}
}

func TestAgentThreadTitleFlattensMrkdwn(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> look at <#C1> &amp; <https://x.test|the link>")

	if len(*calls) != 1 {
		t.Fatalf("want 1 report, got %+v", *calls)
	}
	if want := "#z-claude-dreams @Claude look at #z-claude-dreams & the link"; (*calls)[0].title != want {
		t.Errorf("title = %q, want %q", (*calls)[0].title, want)
	}
}

func TestAgentThreadDetectedAfterPermalinkBackfill(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	root := messages.MessageItem{TS: "100.0", Text: "<@UBOT> hello", UserID: "UHUMAN", ThreadTS: "100.0"}
	a.SetThreadService(NewThreadService(ThreadServiceFuncs{
		CacheRead: func(ids.ChannelID, ids.ThreadTS) []messages.MessageItem {
			return []messages.MessageItem{root}
		},
	}))

	// Permalink-style open: the parent is a stub with no text yet, so
	// detection finds nothing at open time.
	stub := messages.MessageItem{TS: "100.0"}
	a.threadPanel.SetThread(stub, nil, "C1", "100.0")
	a.threadVisible = true
	a.updateAgentThread(stub, "C1", "100.0")
	if len(*calls) != 0 {
		t.Fatalf("stub parent must not detect; calls=%+v", *calls)
	}

	// The fetch lands, the reducer backfills the real parent from cache,
	// and detection re-runs against it.
	reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{}})
	if len(*calls) != 1 || (*calls)[0].agent != "slack-claude" {
		t.Fatalf("backfill must re-run detection; calls=%+v", *calls)
	}
}

func TestAgentThreadTrackingSurvivesAutoHide(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	// Narrow enough that layout.Compute cannot fit the thread pane, so
	// View() auto-hides it. Tracking must survive: an unread reply after
	// the hide still has a sidebar row to land on.
	a.width, a.height = 40, 20
	_ = a.View()
	if a.threadVisible {
		t.Fatal("precondition: thread should auto-hide at width 40")
	}
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "done"})
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("unread after auto-hide must report; got %+v", *unreads)
	}
}

func TestAgentThreadReplyReloadDoesNotStompWorkingState(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	*calls = nil

	// A replies reload re-enters setThreadPanel with the same parent;
	// unchanged detection must not re-report and reset the sidebar to idle.
	reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{{TS: "101.0", ThreadTS: "100.0", Text: "reply"}}})
	if len(*calls) != 0 {
		t.Fatalf("unchanged detection must not re-report; calls=%+v", *calls)
	}
}

func TestAgentThreadIgnoresHumanAndUncachedMentions(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UHUMAN> and <@USTRANGER> chatting")

	if len(*calls) != 0 {
		t.Errorf("non-bot thread must not report; calls=%+v", *calls)
	}
}

func TestAgentThreadTrackingIsSticky(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*calls = nil

	// Neither a non-agent thread nor a close ends tracking: the entry
	// keeps mirroring the last agent thread so unread state and statuses
	// still land.
	openAgentThread(a, "no mentions here")
	a.CloseThread()
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	if len(*calls) != 1 || !(*calls)[0].working {
		t.Fatalf("tracked thread must keep reporting after navigate-away and close; calls=%+v", *calls)
	}
}

func TestAgentThreadReplacedByNewAgentThread(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "done"})
	*calls = nil
	*unreads = nil

	// A different agent thread replaces the entry and starts read: the
	// old thread's unread count must not leak into the new one.
	parent := messages.MessageItem{TS: "200.0", Text: "<@UBOT> next task", UserID: "UHUMAN"}
	a.threadPanel.SetThread(parent, nil, "C1", "200.0")
	a.updateAgentThread(parent, "C1", "200.0")
	if len(*calls) != 1 || (*calls)[0].working || (*calls)[0].status != "" {
		t.Fatalf("replacement must report a fresh idle entry; calls=%+v", *calls)
	}
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "200.0", BotUserID: "UBOT", Status: ""})
	if got := (*calls)[1].status; got != "" {
		t.Errorf("stale unread count leaked into the new thread: %q", got)
	}
}

func TestAgentThreadStatusTransitions(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*calls = nil

	// A status for some other thread is swallowed without reporting.
	if _, handled := reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "999.0", BotUserID: "UBOT", Status: "is thinking…"}); !handled {
		t.Fatal("reducer must claim AssistantStatusMsg")
	}
	// So is a status from a different bot in the open thread.
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UOTHERBOT", Status: "is thinking…"})
	if len(*calls) != 0 {
		t.Fatalf("status for another thread or bot must not report; calls=%+v", *calls)
	}

	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})

	if len(*calls) != 2 {
		t.Fatalf("want 2 reports, got %+v", *calls)
	}
	if !(*calls)[0].working || (*calls)[0].status != "is thinking…" {
		t.Errorf("set: %+v", (*calls)[0])
	}
	if (*calls)[1].working || (*calls)[1].status != "" {
		t.Errorf("clear: %+v", (*calls)[1])
	}
}

func TestAgentThreadUnreadReplies(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	reply := func(ts, user string) messages.MessageItem {
		return messages.MessageItem{TS: ts, ThreadTS: "100.0", UserID: user, Text: "x"}
	}
	// Non-matches: another thread, the parent's own echo, a self reply
	// from another Slack client.
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "999.0", ThreadTS: "999.0", UserID: "UBOT"})
	a.noteAgentThreadReply("", "C2", reply("101.0", "UBOT"))
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "100.0", ThreadTS: "100.0", UserID: "UBOT"})
	a.noteAgentThreadReply("", "C1", reply("101.0", "USELF"))
	if len(*unreads) != 0 {
		t.Fatalf("no unread report expected; got %+v", *unreads)
	}

	a.noteAgentThreadReply("", "C1", reply("102.0", "UBOT"))
	a.noteAgentThreadReply("", "C1", reply("103.0", "UHUMAN"))
	if len(*unreads) != 2 {
		t.Fatalf("want 2 unread reports, got %+v", *unreads)
	}
	if (*unreads)[0].status != "1 unread reply" || (*unreads)[1].status != "2 unread replies" {
		t.Errorf("unread statuses: %+v", *unreads)
	}
	if (*unreads)[0].agent != "slack-claude" || (*unreads)[0].displayName != "Claude" {
		t.Errorf("unread identity: %+v", (*unreads)[0])
	}
}

func TestAgentThreadReadClearsUnread(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*calls = nil

	// Read-state changes for other threads are ignored; the tracked
	// thread's read mark clears the row's status text through a plain
	// idle report (never another completion).
	a.markAgentThreadRead("", "C1", "999.0")
	if len(*calls) != 0 {
		t.Fatalf("other thread's read mark must not report; calls=%+v", *calls)
	}
	a.markAgentThreadRead("", "C1", "100.0")
	if len(*calls) != 1 || (*calls)[0].working || (*calls)[0].status != "" {
		t.Fatalf("read mark must clear via one idle report; calls=%+v", *calls)
	}
	// Idempotent: already-read marks don't re-report.
	a.markAgentThreadRead("", "C1", "100.0")
	if len(*calls) != 1 {
		t.Errorf("second read mark must be a no-op; calls=%+v", *calls)
	}

	// A remote unread mark (boundary, no count) shows at least one.
	*unreads = nil
	a.markAgentThreadUnread("", "C1", "100.0", "101.0")
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("remote unread mark must report; got %+v", *unreads)
	}
	// Already unread: another unread mark must not re-report.
	a.markAgentThreadUnread("", "C1", "100.0", "101.0")
	if len(*unreads) != 1 {
		t.Errorf("second unread mark must be a no-op; got %+v", *unreads)
	}
}

func TestAgentThreadUnreadDeferredWhileWorking(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	*calls = nil

	// Mid-turn, an arriving reply must not publish a completion (it
	// would stomp the live working state); the count rides the turn-end
	// idle report, which is itself the completion edge herdr needs.
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	if len(*unreads) != 0 {
		t.Fatalf("mid-turn reply must not publish a completion; got %+v", *unreads)
	}
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})
	if len(*calls) != 1 || (*calls)[0].working || (*calls)[0].status != "1 unread reply" {
		t.Fatalf("turn end must carry the unread count; calls=%+v", *calls)
	}
}

func TestAgentThreadUnfocusReassertsUnread(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	// Read thread: unfocus reports nothing.
	if _, handled := reduceAgentThread(a, HerdrTabViewMsg{Viewed: false}); !handled {
		t.Fatal("reducer must claim HerdrTabViewMsg")
	}
	if len(*unreads) != 0 {
		t.Fatalf("unfocus while read must not report; got %+v", *unreads)
	}

	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*unreads = nil
	reduceAgentThread(a, HerdrTabViewMsg{Viewed: false})
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("unfocus while unread must re-assert; got %+v", *unreads)
	}
}

func TestAgentThreadReopenMarksRead(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*calls = nil

	// Re-opening the thread lands ThreadRepliesLoadedMsg, whose mark-read
	// must clear the tracked unread state too.
	reduceThreads(a, ThreadRepliesLoadedMsg{ThreadTS: "100.0", Replies: []messages.MessageItem{{TS: "101.0", ThreadTS: "100.0", Text: "x"}}})
	if len(*calls) != 1 || (*calls)[0].status != "" || (*calls)[0].working {
		t.Fatalf("reopen must clear unread via idle report; calls=%+v", *calls)
	}
}

func TestAgentThreadDerivedFieldDriftKeepsState(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*calls = nil
	*unreads = nil

	// The channel name resolves late, so the derived title changes while
	// the thread identity does not. That must refresh the row in place,
	// never read as a thread switch: a switch would reset the live
	// working state and drop the pending unread count, and the resulting
	// working→idle edge would light the dot mid-turn.
	a.channelNames["C1"] = "eng-agents"
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> hi", UserID: "UHUMAN"}
	a.updateAgentThread(parent, "C1", "100.0")

	if len(*unreads) != 0 {
		t.Fatalf("a title refresh must not publish a completion; got %+v", *unreads)
	}
	if len(*calls) != 1 {
		t.Fatalf("want 1 refresh report, got %+v", *calls)
	}
	if c := (*calls)[0]; !c.working || c.status != "is thinking…" || c.title != "#eng-agents @Claude hi" {
		t.Errorf("refresh must keep working state and carry the new title: %+v", c)
	}
	if !a.agentSidebar.working || a.agentSidebar.unreadTotal() != 1 {
		t.Errorf("live state reset by a title refresh: working=%v unread=%d",
			a.agentSidebar.working, a.agentSidebar.unreadTotal())
	}
}

func TestAgentThreadSelfReplyFilterIsPerWorkspace(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")

	// A real workspace switch: both the active workspace and the current
	// user move on, while the tracked thread stays in T1. A self reply
	// arriving for it (posted from another Slack client) carries T1's
	// self id, so the filter has to compare against the id captured when
	// tracking started, not whoever the user is now.
	a.activeTeamID = "T2"
	a.currentUserID = "USELF-IN-T2"
	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "USELF", Text: "mine"})
	if len(*unreads) != 0 {
		t.Fatalf("own reply must never count as unread; got %+v", *unreads)
	}

	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "102.0", ThreadTS: "100.0", UserID: "UBOT", Text: "theirs"})
	if len(*unreads) != 1 {
		t.Fatalf("someone else's reply must count; got %+v", *unreads)
	}
}
func TestAgentThreadTurnStateDroppedOnConnectionChange(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	*calls = nil

	// The turn-end status is edge-triggered: a WS drop takes it with it.
	// A connection-state change drops the latch, with nothing published
	// while no unread replies are waiting (an idle report here would be a
	// completion edge, lighting the dot for a turn nobody saw end).
	reduceIO(a, ConnectionStateMsg{TeamID: a.activeTeamID, State: int(statusbar.StateReconnecting)})
	if len(*calls) != 0 || len(*unreads) != 0 {
		t.Fatalf("dropping the latch must publish nothing when read; calls=%+v unreads=%+v", *calls, *unreads)
	}
	if a.agentSidebar.working {
		t.Fatal("working latch survived a connection-state change")
	}

	// With the latch cleared, unread publication works again instead of
	// being swallowed forever.
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("unread must publish after the latch drops; got %+v", *unreads)
	}
}

func TestAgentThreadTurnDropPublishesPendingUnread(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	if len(*unreads) != 0 {
		t.Fatalf("precondition: mid-turn reply defers publication; got %+v", *unreads)
	}

	// The reply deferred during the turn is published when the drop makes
	// it publishable, rather than waiting for a turn end that never comes.
	reduceIO(a, ConnectionStateMsg{TeamID: a.activeTeamID, State: int(statusbar.StateConnected)})
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("deferred unread must publish on the drop; got %+v", *unreads)
	}
}

func TestAgentThreadTurnLatchIgnoresOtherWorkspaces(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reduceAgentThread(a, AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})

	// Connection states are workspace-scoped: another workspace's blip
	// says nothing about this thread's assistant, so the turn stands and
	// the deferred unread stays deferred.
	reduceIO(a, ConnectionStateMsg{TeamID: "T2", State: int(statusbar.StateReconnecting)})
	if !a.agentSidebar.working || len(*unreads) != 0 {
		t.Fatalf("another workspace's reconnect dropped the turn; working=%v unreads=%+v",
			a.agentSidebar.working, *unreads)
	}

	reduceIO(a, ConnectionStateMsg{TeamID: "T1", State: int(statusbar.StateReconnecting)})
	if a.agentSidebar.working || len(*unreads) != 1 {
		t.Fatalf("the thread's own workspace must drop the turn; working=%v unreads=%+v",
			a.agentSidebar.working, *unreads)
	}
}

func TestAgentThreadBackgroundListReloadKeepsUnread(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	var fetched int
	a.SetThreadService(NewThreadService(ThreadServiceFuncs{
		CacheRead: func(ids.ChannelID, ids.ThreadTS) []messages.MessageItem { return nil },
		Fetch: func(ids.ChannelID, ids.ThreadTS) tea.Msg {
			fetched++
			return nil
		},
	}))
	a.view = ViewThreads
	a.threadsView.SetSummaries([]cache.ThreadSummary{{
		ChannelID: "C1", ThreadTS: "100.0", ParentTS: "100.0", ParentUserID: "UHUMAN", Unread: true,
	}})
	// The user closed the panel but stayed in the threads view.
	a.CloseThread()
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*unreads = nil

	// A reply schedules a list refresh. Landing it must not resurrect the
	// closed panel: that reopen would fetch and mark the thread read for a
	// reply the user never saw, wiping the unread state it just raised.
	reduceThreads(a, ThreadsListLoadedMsg{TeamID: a.activeTeamID, Summaries: []cache.ThreadSummary{{
		ChannelID: "C1", ThreadTS: "100.0", ParentTS: "100.0", ParentUserID: "UHUMAN", Unread: true,
	}}})
	if fetched != 0 {
		t.Errorf("background reload fetched a closed thread %d times", fetched)
	}
	if a.threadVisible {
		t.Error("background reload resurrected a closed thread panel")
	}
	if a.agentSidebar.unreadTotal() != 1 {
		t.Errorf("unread count wiped by a background reload: %d", a.agentSidebar.unreadTotal())
	}
}

func TestAgentThreadOpenClearsUnreadWhenFetchFails(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	a.SetThreadService(NewThreadService(ThreadServiceFuncs{
		CacheRead: func(ids.ChannelID, ids.ThreadTS) []messages.MessageItem { return nil },
		Fetch:     func(ids.ChannelID, ids.ThreadTS) tea.Msg { return nil },
	}))
	a.view = ViewThreads
	a.threadsView.SetSummaries([]cache.ThreadSummary{{
		ChannelID: "C1", ThreadTS: "100.0", ParentTS: "100.0", ParentUserID: "UHUMAN", Unread: true,
	}})
	a.CloseThread()
	a.lastOpenedChannelID, a.lastOpenedThreadTS = "", ""
	*calls = nil

	// Opening the thread is the read: the sidebar must clear with the
	// threads list, not wait for the follow-up fetch. When that fetch
	// fails (nil replies), its reducer arm returns early, so relying on
	// it left the list showing read while the herdr row still claimed
	// unread and re-lit the dot on the next tab unfocus.
	_ = a.openSelectedThreadCmd(false)
	if a.agentSidebar.unreadTotal() != 0 {
		t.Fatalf("opening the thread must clear the sidebar's unread count, got %d", a.agentSidebar.unreadTotal())
	}
	if len(*calls) != 1 || (*calls)[0].status != "" {
		t.Fatalf("want one idle clear report, got %+v", *calls)
	}
}

func TestAgentThreadTracksItsOwnWorkspaceInBackground(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*unreads = nil

	// The user switches to another workspace. The tracked thread's
	// replies still arrive tagged with T1, and must still count: the
	// hook runs ahead of the reducer's background skip, which exists to
	// spare the panes and sidebar, not thread-scoped consumers.
	a.activeTeamID = "T2"
	reduceSend(a, NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "done",
	}})
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("background reply to the tracked thread must count; got %+v", *unreads)
	}

	// An edit echo is not a new reply.
	reduceSend(a, NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "done (edited)", IsEdited: true,
	}})
	if len(*unreads) != 1 {
		t.Fatalf("edit echo must not bump the count; got %+v", *unreads)
	}

	// A remote read-mark from that same background workspace clears it.
	reduceThreads(a, ThreadMarkedRemoteMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TS: "101.0", Read: true})
	if a.agentSidebar.unreadTotal() != 0 {
		t.Fatalf("background read-mark must clear the count, got %d", a.agentSidebar.unreadTotal())
	}
}

func TestAgentThreadIgnoresOtherWorkspacesSameChannelID(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*unreads = nil

	// Channel and thread ids are only unique within a workspace, and
	// every workspace's traffic now reaches the reducer, so the team tag
	// is what keeps a lookalike thread elsewhere from driving this row.
	reduceSend(a, NewMessageMsg{TeamID: "T2", ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "different workspace",
	}})
	reduceThreads(a, ThreadMarkedRemoteMsg{TeamID: "T2", ChannelID: "C1", ThreadTS: "100.0", TS: "101.0", Read: false})
	if len(*unreads) != 0 || a.agentSidebar.unreadTotal() != 0 {
		t.Fatalf("another workspace's lookalike thread must not drive the row; unreads=%+v count=%d",
			*unreads, a.agentSidebar.unreadTotal())
	}
}

func TestAgentThreadOwnReplyNeverCountsAsUnread(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*unreads = nil
	a.activeTeamID = "T2"

	// Two independent ways a reply can be the user's own, both of which
	// have to be caught here: this hook runs ahead of the reducer's own
	// dedup, so nothing downstream will catch what it lets through.
	reduceSend(a, NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "USELF", Text: "typed elsewhere",
	}})
	if len(*unreads) != 0 {
		t.Fatalf("a reply carrying the thread workspace's own user id must not count; got %+v", *unreads)
	}

	// An slk-originated echo, recognized by the send dedup rather than by
	// author id — which is what covers a reply whose author id doesn't
	// match the snapshot taken when tracking started.
	a.selfSend.RecordSent("102.0")
	reduceSend(a, NewMessageMsg{TeamID: "T1", ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "USOMEOTHERID", Text: "sent from slk",
	}})
	if len(*unreads) != 0 {
		t.Fatalf("an slk-sent reply must not count whatever id it carries; got %+v", *unreads)
	}
}

func TestAgentThreadAssistantStatusIsWorkspaceScoped(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	*calls = nil

	// A same-id thread in another workspace must not claim this row's
	// turn. If it could, its "is thinking…" would latch working here and
	// silently swallow every unread publication for the tracked thread
	// until that other turn ended.
	reduceAgentThread(a, AssistantStatusMsg{
		TeamID: "T2", ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…",
	})
	if len(*calls) != 0 || a.agentSidebar.working {
		t.Fatalf("another workspace's status must not touch the row; calls=%+v working=%v",
			*calls, a.agentSidebar.working)
	}
	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	if len(*unreads) != 1 {
		t.Fatalf("unread publication must not be swallowed; got %+v", *unreads)
	}

	// The tracked thread's own workspace still drives it, including while
	// the user is looking at a different workspace.
	a.activeTeamID = "T2"
	reduceAgentThread(a, AssistantStatusMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…",
	})
	if !a.agentSidebar.working {
		t.Fatal("the tracked thread's own workspace must drive its turn state")
	}
}

func TestAgentThreadRemoteMarkReportsOnce(t *testing.T) {
	a, calls, _ := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*calls = nil

	// One mark, one report. This pins the invariant rather than the
	// structure: with the redundant pre-skip call still in place the
	// second one short-circuits, so no test can tell the two shapes
	// apart today -- which is the reason the redundancy went, before
	// something made the calls non-idempotent.
	reduceThreads(a, ThreadMarkedRemoteMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TS: "101.0", Read: true,
	})
	if len(*calls) != 1 {
		t.Fatalf("want exactly one report for one mark, got %+v", *calls)
	}
	if a.agentSidebar.unreadTotal() != 0 {
		t.Fatalf("mark must clear the count, got %d", a.agentSidebar.unreadTotal())
	}
}

func TestAgentThreadDeletedReplyStopsCounting(t *testing.T) {
	a, calls, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	reply := func(ts string) messages.MessageItem {
		return messages.MessageItem{TS: ts, ThreadTS: "100.0", UserID: "UBOT", Text: "x"}
	}
	a.noteAgentThreadReply("T1", "C1", reply("101.0"))
	a.noteAgentThreadReply("T1", "C1", reply("102.0"))
	*calls, *unreads = nil, nil

	// Deleting one counted reply leaves the other, and republishes so the
	// row's text stops overstating what is waiting.
	reduceSend(a, WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "102.0"})
	if got := a.agentSidebar.unreadTotal(); got != 1 {
		t.Fatalf("one deletion should leave one unread, got %d", got)
	}
	if len(*unreads) != 1 || (*unreads)[0].status != "1 unread reply" {
		t.Fatalf("deletion must republish the reduced count; got %+v", *unreads)
	}

	// Deleting something never counted (the parent, an already-read
	// reply) must not touch the row.
	*unreads = nil
	reduceSend(a, WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "999.0"})
	if len(*unreads) != 0 || a.agentSidebar.unreadTotal() != 1 {
		t.Fatalf("deleting an uncounted message must be inert; unreads=%+v total=%d",
			*unreads, a.agentSidebar.unreadTotal())
	}

	// Deleting the last one leaves the thread read, cleared through a
	// plain idle report rather than another completion.
	*calls = nil
	reduceSend(a, WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "101.0"})
	if a.agentSidebar.unreadTotal() != 0 {
		t.Fatalf("deleting the last unread reply must clear the row, got %d", a.agentSidebar.unreadTotal())
	}
	if len(*calls) != 1 || (*calls)[0].status != "" || (*calls)[0].working {
		t.Fatalf("want one idle clear report, got %+v", *calls)
	}
}

func TestAgentThreadDeletionFromBackgroundWorkspace(t *testing.T) {
	a, _, unreads := newAgentTestApp()
	openAgentThread(a, "<@UBOT> hi")
	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "x"})
	*unreads = nil

	// The deletion that matters most is the one the user can't see: the
	// tracked thread is in T1, the user is in T2, so nothing on screen
	// reconciles a retracted reply against the row.
	a.activeTeamID = "T2"
	reduceSend(a, WSMessageDeletedMsg{TeamID: "T1", ChannelID: "C1", TS: "101.0"})
	if got := a.agentSidebar.unreadTotal(); got != 0 {
		t.Fatalf("background deletion must retract the count, got %d", got)
	}

	// A deletion in some other workspace that happens to share the
	// channel and message ids must not.
	a.noteAgentThreadReply("T1", "C1", messages.MessageItem{TS: "102.0", ThreadTS: "100.0", UserID: "UBOT", Text: "y"})
	reduceSend(a, WSMessageDeletedMsg{TeamID: "T3", ChannelID: "C1", TS: "102.0"})
	if got := a.agentSidebar.unreadTotal(); got != 1 {
		t.Fatalf("another workspace's deletion must be inert, got %d", got)
	}
}

func TestPaneViewedDefaultsToVisibleWithoutHerdr(t *testing.T) {
	a, _, _ := newAgentTestApp()

	// Outside herdr nothing reports viewedness, and the honest default
	// is that the user can see the pane: a reader that gates on this
	// must not withhold behavior for want of a signal that will never
	// arrive.
	if !a.PaneViewed() {
		t.Fatal("PaneViewed must be true when nothing reports it")
	}

	reduceAgentThread(a, HerdrTabViewMsg{Viewed: false})
	if a.PaneViewed() {
		t.Fatal("a report of unviewed must be believed")
	}
	reduceAgentThread(a, HerdrTabViewMsg{Viewed: true})
	if !a.PaneViewed() {
		t.Fatal("a report of viewed must be believed")
	}
}
