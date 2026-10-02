package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// The !review-link reply O was reported on, as Slack sends its text: a
// permalink into a thread of the same channel (thread_ts), then a
// permalink to a top-level message of another channel that nobody has
// replied to yet.
const (
	reviewLinkText = "!review-link\n" +
		"annotation: <https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1790979381013049?thread_ts=1790910827.409349&amp;cid=C0BCG30UGEP>\n" +
		"thread: <https://colony-pyo1658.slack.com/archives/C0C5TD1LLQH/p1790979429246289>\n" +
		"Orz 2 Sg P 1M · <https://claude.ai/claude-in-slack/T0AHVJD24BV/C0BCG30UGEP/configure|Configure>"
	reviewLinkThreadTS = "1790910827.409349"
	reviewLinkReplyTS  = "1790979457.552669"
	judgeChannelID     = "C0C5TD1LLQH"
	judgePostTS        = "1790979429.246289"
)

// judgeHistory is the judge channel's history, the linked post without
// replies.
func judgeHistory() []messages.MessageItem {
	return []messages.MessageItem{
		{TS: "1790979234.579699", Text: "!review-open (an older one)", ReplyCount: 1},
		{TS: judgePostTS, Text: "!review-open"},
	}
}

// reviewLinkApp is slk outside herdr (O navigates in place), reading the
// thread that holds the !review-link reply, cursor on that reply.
// threadFetches counts the thread opens from here on.
func reviewLinkApp(t *testing.T) (a *App, threadFetches *int) {
	t.Helper()
	a = newTestApp(t, withSize(200, 60), withActiveTeam("T1"))
	a.workspaceDomains["T1"] = "colony-pyo1658"
	a.setChannelLookupFuncForTest(func(id ids.ChannelID) (string, string, bool) {
		switch id {
		case "C0BCG30UGEP":
			return "eng", "channel", true
		case judgeChannelID:
			return "z-annotation-testing", "channel", true
		}
		return "", "", false
	})
	threadFetches = new(int)
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		*threadFetches++
		return nil
	})
	a.activeChannelID = "C0BCG30UGEP"
	parent := messages.MessageItem{TS: reviewLinkThreadTS, ThreadTS: reviewLinkThreadTS, Text: "parent", ReplyCount: 1}
	a.messagepane.SetMessages([]messages.MessageItem{parent})
	a.messagepane.SelectByTS(parent.TS)
	drainCmd(a.openThreadForSelectedMessage())
	_, _ = a.Update(ThreadRepliesLoadedMsg{ThreadTS: reviewLinkThreadTS, Replies: []messages.MessageItem{
		{TS: reviewLinkReplyTS, ThreadTS: reviewLinkThreadTS, Text: reviewLinkText},
	}})
	a.threadPanel.SelectByTS(reviewLinkReplyTS)
	*threadFetches = 0
	return a, threadFetches
}

// followThreadLink presses key (o or O) on the !review-link reply, picks
// its "thread:" row in the picker, and lands the cross-channel nav: the
// channel switch, then the loaded history.
func followThreadLink(t *testing.T, a *App, key rune) {
	t.Helper()
	a.handleNormalMode(tea.KeyPressMsg{Code: key, Text: string(key)})
	if a.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want the link picker", a.mode)
	}
	a.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	open, ok := a.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})().(OpenLinkMsg)
	if !ok {
		t.Fatal("Enter in the picker gave no OpenLinkMsg")
	}
	_, cmd := a.Update(open)
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != judgeChannelID {
		t.Fatalf("ChannelSelectedMsg = %+v ok=%v, want %s", sel, ok, judgeChannelID)
	}
	_, cmd = a.Update(sel)
	drainCmd(cmd)
	_, cmd = a.Update(MessagesLoadedMsg{ChannelID: judgeChannelID, Messages: judgeHistory()})
	drainCmd(cmd)
}

func wantThreadOpenOnJudgePost(t *testing.T, a *App) {
	t.Helper()
	if !a.threadVisible || a.threadPanel.ThreadTS() != judgePostTS {
		t.Fatalf("threadVisible = %v on thread %q, want the thread panel open on %s", a.threadVisible, a.threadPanel.ThreadTS(), judgePostTS)
	}
	if a.focusedPanel != PanelThread {
		t.Errorf("focusedPanel = %v, want PanelThread", a.focusedPanel)
	}
	if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != judgePostTS {
		t.Errorf("thread cursor = %+v, want the linked post %s", sel, judgePostTS)
	}
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != judgePostTS {
		t.Errorf("channel cursor = %+v ok=%v, want the linked post %s", sel, ok, judgePostTS)
	}
	if a.pendingLinkNav != nil {
		t.Errorf("pendingLinkNav not retired: %+v", a.pendingLinkNav)
	}
}

// O on a permalink to a top-level message nobody has replied to opens
// that message's thread panel, ready for the first reply.
func TestShiftO_MessageWithoutRepliesLink_OpensItsThread(t *testing.T) {
	a, threadFetches := reviewLinkApp(t)
	followThreadLink(t, a, 'O')
	wantThreadOpenOnJudgePost(t, a)
	if *threadFetches != 1 {
		t.Errorf("thread opened %d times, want 1", *threadFetches)
	}
}

// o on the same link stops at selecting the message in its channel.
func TestO_MessageWithoutRepliesLink_SelectsInChannel(t *testing.T) {
	a, threadFetches := reviewLinkApp(t)
	followThreadLink(t, a, 'o')
	if a.threadVisible || *threadFetches != 0 {
		t.Errorf("threadVisible = %v after %d thread opens, want no thread panel", a.threadVisible, *threadFetches)
	}
	if a.focusedPanel != PanelMessages {
		t.Errorf("focusedPanel = %v, want PanelMessages", a.focusedPanel)
	}
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != judgePostTS {
		t.Errorf("channel cursor = %+v ok=%v, want the linked post %s", sel, ok, judgePostTS)
	}
}

// `slk <link>`, which is also what O runs in its new herdr tab: the
// startup link to a message without replies opens its thread panel. The
// thread opens once, on the cached render, and the history that lands
// after it leaves the panel alone.
func TestStartupLink_MessageWithoutReplies_OpensItsThread(t *testing.T) {
	a := newTestApp(t, withSize(200, 60), withChannelService(core.ChannelServiceFuncs{
		ReadCache: func(ids.ChannelID) []messages.MessageItem { return judgeHistory() },
	}))
	threadFetches := 0
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		threadFetches++
		return nil
	})
	a.SetStartupLink(judgeChannelID, judgePostTS, "")
	_, cmd := a.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		InitialActive: true,
		Channels:      []sidebar.ChannelItem{{ID: judgeChannelID, Name: "z-annotation-testing", Type: "channel"}},
		FinderItems:   []channelfinder.Item{{ID: judgeChannelID, Name: "z-annotation-testing", Type: "channel", Joined: true}},
	})
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != judgeChannelID {
		t.Fatalf("ChannelSelectedMsg = %+v ok=%v, want %s", sel, ok, judgeChannelID)
	}
	_, cmd = a.Update(sel)
	drainCmd(cmd)
	if !a.threadVisible {
		t.Error("thread panel not open on the cached render")
	}
	_, cmd = a.Update(MessagesLoadedMsg{ChannelID: judgeChannelID, Messages: judgeHistory()})
	drainCmd(cmd)
	wantThreadOpenOnJudgePost(t, a)
	if threadFetches != 1 {
		t.Errorf("thread opened %d times, want 1", threadFetches)
	}

	// Nothing cached and the post older than the loaded history: the
	// window fetched around it finishes the same way.
	threadFetches = 0
	a = newTestApp(t, withSize(200, 60))
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		threadFetches++
		return nil
	})
	a.activeChannelID = judgeChannelID
	a.SetStartupLink(judgeChannelID, judgePostTS, "")
	a.pendingLinkNav, a.startupLinkNav = a.startupLinkNav, nil
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: judgeChannelID, Messages: []messages.MessageItem{{TS: "1790990000.000100", Text: "newer"}}})
	if a.pendingLinkNav == nil {
		t.Fatal("nav retired before the window around the post landed")
	}
	_, cmd = a.Update(MessagesAroundLoadedMsg{ChannelID: judgeChannelID, TargetTS: judgePostTS, Messages: judgeHistory()})
	drainCmd(cmd)
	wantThreadOpenOnJudgePost(t, a)
	if threadFetches != 1 {
		t.Errorf("off-buffer: thread opened %d times, want 1", threadFetches)
	}
}

// A startup link without thread_ts to a reply that was also sent to the
// channel opens the reply's thread, cursor on the reply.
func TestStartupLink_BroadcastReply_OpensItsParentsThread(t *testing.T) {
	const replyTS = "1790979500.000100"
	a := newTestApp(t, withSize(200, 60))
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg { return nil })
	a.activeChannelID = judgeChannelID
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: judgePostTS, ThreadTS: judgePostTS, Text: "!review-open", ReplyCount: 1},
		{TS: replyTS, ThreadTS: judgePostTS, Subtype: "thread_broadcast", Text: "verdict"},
	})
	a.SetStartupLink(judgeChannelID, replyTS, "")
	a.pendingLinkNav, a.startupLinkNav = a.startupLinkNav, nil
	drainCmd(a.completePendingLinkNav(judgeChannelID, true))
	if !a.threadVisible || a.threadPanel.ThreadTS() != judgePostTS {
		t.Fatalf("threadVisible = %v on thread %q, want thread %s", a.threadVisible, a.threadPanel.ThreadTS(), judgePostTS)
	}
	_, _ = a.Update(ThreadRepliesLoadedMsg{ThreadTS: judgePostTS, Replies: []messages.MessageItem{
		{TS: replyTS, ThreadTS: judgePostTS, Text: "verdict"},
	}})
	if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != replyTS {
		t.Errorf("thread cursor = %+v, want the linked reply %s", sel, replyTS)
	}
}

// A workspace-search jump to a message without replies stays a select.
func TestSearchNav_MessageWithoutReplies_SelectsInChannel(t *testing.T) {
	a := newTestApp(t, withSize(200, 60))
	a.activeChannelID = judgeChannelID
	a.messagepane.SetMessages(judgeHistory())
	a.pendingLinkNav = &pendingLinkNav{channelID: judgeChannelID, messageTS: judgePostTS}
	drainCmd(a.completePendingLinkNav(judgeChannelID, true))
	if a.threadVisible {
		t.Error("search jump opened a thread panel")
	}
}
