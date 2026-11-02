package ui

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// threadsPage returns n summaries, newest first, whose LastReplyTS
// count down from start.
func threadsPage(start, n int) []core.ThreadSummary {
	out := make([]core.ThreadSummary, n)
	for i := range out {
		ts := fmt.Sprintf("%d.000000", start-i)
		out[i] = core.ThreadSummary{ChannelID: "C1", ThreadTS: ts, ParentTS: ts, LastReplyTS: ts}
	}
	return out
}

// threadsFetchRecorder wires a ThreadService whose ListFetch records
// each cursor it is asked for and returns nothing.
func threadsFetchRecorder(app *App) *[]string {
	var cursors []string
	app.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		ListFetch: func(teamID ids.TeamID, cursor string) core.Msg {
			cursors = append(cursors, string(teamID)+"@"+cursor)
			return nil
		},
	}))
	return &cursors
}

// Opening the Threads view asks Slack for its first page, every time
// it opens, as the Slack app does; boot asks for nothing.
func TestApp_ThreadsViewOpenFetchesFirstPage(t *testing.T) {
	app := NewApp()
	cursors := threadsFetchRecorder(app)
	_, cmd := app.Update(WorkspaceReadyMsg{TeamID: "T1", TeamName: "Test", InitialActive: true})
	drainBatch(cmd)
	if len(*cursors) != 0 {
		t.Fatalf("workspace-ready fetched %v; boot must not load the Threads view", *cursors)
	}

	app.activeTeamID = "T1"
	for range 2 {
		_, cmd = app.Update(ThreadsViewActivatedMsg{})
		drainBatch(cmd)
	}
	if want := []string{"T1@", "T1@"}; !slices.Equal(*cursors, want) {
		t.Errorf("fetches = %v, want %v: the first page on each open", *cursors, want)
	}
}

// Moving toward the end of the loaded rows fetches the next page once,
// at the cursor the last page named, and the page lands below.
func TestApp_ThreadsViewLoadsTheNextPageNearTheEnd(t *testing.T) {
	app := NewApp()
	app.activeTeamID = "T1"
	app.view = ViewThreads
	app.focusedPanel = PanelMessages
	cursors := threadsFetchRecorder(app)
	app.Update(ThreadsListLoadedMsg{TeamID: "T1", Summaries: threadsPage(100, 8), NextCursor: "93.000000"})
	*cursors = nil

	app.Update(keyCode('j'))
	if len(*cursors) != 0 {
		t.Fatalf("fetches = %v after one row; want none until near the end", *cursors)
	}
	for range 6 {
		_, cmd := app.Update(keyCode('j'))
		for _, c := range drainBatchCmds(cmd) {
			if c != nil {
				c()
			}
		}
	}
	if want := []string{"T1@93.000000"}; !slices.Equal(*cursors, want) {
		t.Fatalf("fetches = %v, want %v: the next page, once", *cursors, want)
	}

	app.Update(ThreadsListLoadedMsg{TeamID: "T1", Cursor: "93.000000", Summaries: threadsPage(92, 8)})
	got := app.threadsView.Summaries()
	if len(got) != 16 || got[8].ThreadTS != "92.000000" {
		t.Errorf("rows = %d (row 8 %q), want 16 with the page appended", len(got), got[min(8, len(got)-1)].ThreadTS)
	}
}

// The sidebar's Threads badge is Slack's count, per workspace; opening
// an unread row moves it at once, ahead of Slack's next count.
func TestApp_ThreadsBadgeFollowsSlack(t *testing.T) {
	app := NewApp()
	app.activeTeamID = "T1"
	app.Update(ThreadsBadgeUpdatedMsg{TeamID: "T1", UnreadThreads: 5})
	app.Update(ThreadsBadgeUpdatedMsg{TeamID: "T2", UnreadThreads: 9})
	if got := app.sidebar.ThreadsUnreadCount(); got != 5 {
		t.Fatalf("badge = %d, want the active workspace's 5", got)
	}

	app.view = ViewThreads
	app.threadsView.SetSummaries([]core.ThreadSummary{{ChannelID: "C1", ThreadTS: "1.0", ParentTS: "1.0", LastReplyTS: "2.0", Unread: true}})
	app.openSelectedThreadCmd(false)
	if got := app.sidebar.ThreadsUnreadCount(); got != 4 {
		t.Errorf("badge after opening an unread row = %d, want 4", got)
	}

	// A loaded page does not recount it: the page holds a few threads
	// of many.
	app.Update(ThreadsListLoadedMsg{TeamID: "T1", Summaries: threadsPage(10, 2)})
	if got := app.sidebar.ThreadsUnreadCount(); got != 4 {
		t.Errorf("badge after a page load = %d, want it left at 4", got)
	}
}

// A thread opened with no cursor known gets its "── new ──" boundary
// from the cursor the replies fetch recorded, before the mark moves it.
func TestThreadRepliesLoaded_AppliesBoundaryRecordedByTheFetch(t *testing.T) {
	app := NewApp()
	cursor := ""
	app.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		ThreadLastRead: func(ids.ChannelID, ids.ThreadTS) string { return cursor },
		Mark: func(ids.ChannelID, ids.ThreadTS, ids.MessageTS) core.Cmd {
			cursor = "R5" // the mark moves the cursor
			return nil
		},
	}))
	app.threadVisible = true
	app.threadPanel.SetThread(messages.MessageItem{TS: "P1"}, nil, "C1", "P1")
	app.applyThreadUnreadBoundary("C1", "P1")

	cursor = "R1" // what the replies fetch recorded from the root
	app.Update(ThreadRepliesLoadedMsg{
		ThreadTS: "P1",
		Replies:  []messages.MessageItem{{TS: "R1"}, {TS: "R5"}},
	})
	if got := app.threadPanel.LastReadTS(); got != "R1" {
		t.Errorf("boundary = %q, want R1 from the fetch, ahead of the mark", got)
	}
}

// threadOpenRecorder wires a ThreadService whose ListFetch serves pages
// from pages (by cursor) and whose Fetch and Mark record what was
// opened and marked: either is Slack being told a thread was read.
func threadOpenRecorder(app *App, pages func(cursor string) ThreadsListLoadedMsg) (opened, marked *[]string) {
	opened, marked = new([]string), new([]string)
	app.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		ListFetch: func(teamID ids.TeamID, cursor string) core.Msg {
			m := pages(cursor)
			m.TeamID, m.Cursor = string(teamID), cursor
			return m
		},
		Fetch: func(channelID ids.ChannelID, threadTS ids.ThreadTS) core.Msg {
			*opened = append(*opened, string(threadTS))
			return nil
		},
		Mark: func(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) core.Cmd {
			*marked = append(*marked, string(threadTS))
			return nil
		},
	}))
	return opened, marked
}

// The cascade a live check hit: Slack lists unread threads first, so
// the thread the user opened and read leaves the first page on the
// next refresh. The refresh must leave the selection on it and open
// nothing; before, the selection fell to the top row and the page load
// opened it, marking it read, whose refresh opened the next, and so on
// down every unread thread.
func TestThreadsRefresh_ReadThreadLeavingFirstPageOpensNothing(t *testing.T) {
	app := NewApp()
	app.activeTeamID = "T1"
	app.view = ViewThreads
	app.focusedPanel = PanelMessages
	app.threadsDirtyDebounce = time.Millisecond
	unread := func(ts, latest string) core.ThreadSummary {
		return core.ThreadSummary{ChannelID: "C1", ThreadTS: ts, ParentTS: ts, LastReplyTS: latest, Unread: true}
	}
	// Unread first, then read; the cursor is the last row's latest_reply.
	first := []core.ThreadSummary{unread("1.0", "50.0"), unread("2.0", "40.0"), unread("3.0", "30.0")}
	refreshed := []core.ThreadSummary{unread("2.0", "40.0"), unread("3.0", "30.0"), unread("4.0", "20.0")}
	served := first
	opened, marked := threadOpenRecorder(app, func(cursor string) ThreadsListLoadedMsg {
		if cursor != "" {
			return ThreadsListLoadedMsg{} // the read threads: none here
		}
		return ThreadsListLoadedMsg{Summaries: served, NextCursor: served[len(served)-1].LastReplyTS}
	})
	app.Update(ThreadsListLoadedMsg{TeamID: "T1", Summaries: first, NextCursor: "30.0"})

	// The user opens the top thread and reads it.
	feed(t, app, app.openSelectedThreadCmd(false), 0)
	app.threadsView.MarkByThreadTSReadAt("C1", "1.0", "50.0")
	*opened, *marked = nil, nil

	// Slack's update_global_thread_state for the read: a debounced
	// first-page refresh, which no longer holds thread 1.0.
	served = refreshed
	_, cmd := app.Update(ThreadsListDirtyMsg{TeamID: "T1"})
	feed(t, app, cmd, 0)

	if len(*opened) != 0 || len(*marked) != 0 {
		t.Errorf("refresh opened %v and marked %v; want nothing", *opened, *marked)
	}
	if _, ts, _ := app.threadsView.Selected(); ts != "1.0" {
		t.Errorf("selection = %s, want it left on the thread being read", ts)
	}
	if got := app.threadPanel.ThreadTS(); got != "1.0" {
		t.Errorf("thread panel = %s, want 1.0 still open", got)
	}
}

// A page landing never opens a thread, on the open of the view or
// after: only the user opens one, and opening one marks it read.
func TestThreadsPageLoad_OpensNoThread(t *testing.T) {
	app := NewApp()
	app.activeTeamID = "T1"
	app.focusedPanel = PanelMessages
	page := []core.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "1.0", ParentTS: "1.0", LastReplyTS: "5.0", Unread: true},
		{ChannelID: "C1", ThreadTS: "2.0", ParentTS: "2.0", LastReplyTS: "4.0", Unread: true},
	}
	opened, marked := threadOpenRecorder(app, func(cursor string) ThreadsListLoadedMsg {
		return ThreadsListLoadedMsg{Summaries: page}
	})

	_, cmd := app.Update(ThreadsViewActivatedMsg{})
	feed(t, app, cmd, 0)

	if len(app.threadsView.Summaries()) != 2 {
		t.Fatalf("rows = %d, want the page loaded", len(app.threadsView.Summaries()))
	}
	if len(*opened) != 0 || len(*marked) != 0 || app.threadVisible {
		t.Errorf("page load opened %v, marked %v (panel open %v); want nothing", *opened, *marked, app.threadVisible)
	}
}

func TestThreadMarkedLocalMsg_SuccessAppliesCursor(t *testing.T) {
	app := NewApp()
	app.threadsView.SetSummaries([]core.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: true},
	})

	_, _ = app.Update(ThreadMarkedLocalMsg{ChannelID: "C1", ThreadTS: "P1", TS: "5.000000"})

	for _, s := range app.threadsView.Summaries() {
		if s.ThreadTS == "P1" && s.Unread {
			t.Error("a successful thread mark must clear the unread flag")
		}
	}
}

func TestThreadMarkedLocalMsg_FailureLeavesFlagAlone(t *testing.T) {
	app := NewApp()
	app.threadsView.SetSummaries([]core.ThreadSummary{
		{ChannelID: "C1", ThreadTS: "P1", LastReplyTS: "5.000000", Unread: true},
	})

	_, _ = app.Update(ThreadMarkedLocalMsg{
		ChannelID: "C1", ThreadTS: "P1", TS: "5.000000",
		Err: errors.New("subscriptions.thread.mark: thread_not_found"),
	})

	for _, s := range app.threadsView.Summaries() {
		if s.ThreadTS == "P1" && !s.Unread {
			t.Error("a rejected thread mark must leave the thread unread")
		}
	}
}

// markSentinelMsg stands in for the ThreadMarkedLocalMsg the production
// Mark cmd yields, so the test can prove the reducer actually propagated
// the cmd rather than merely calling Mark for its side effect.
type markSentinelMsg struct{}

// The cmd returned by Mark is the entire delivery mechanism for this
// task: it is what persists the read cursor and hands
// ThreadMarkedLocalMsg back to the reducer. If the reducer drops it,
// MarkThread still fires but the cursor is never written locally and
// slk regresses to the stale-cursor bug. Asserting only that Mark was
// called cannot see that, so this asserts the cmd reaches the caller
// and yields the sentinel.
func TestThreadRepliesLoaded_ReturnsMarkCmd(t *testing.T) {
	app := NewApp()
	var got []string
	app.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		Mark: func(channelID ids.ChannelID, threadTS ids.ThreadTS, ts ids.MessageTS) core.Cmd {
			got = append(got, string(channelID)+"/"+string(threadTS)+"/"+string(ts))
			return func() core.Msg { return markSentinelMsg{} }
		},
	}))
	app.threadVisible = true
	app.threadPanel.SetThread(messages.MessageItem{TS: "P1"}, nil, "C1", "P1")

	_, cmd := app.Update(ThreadRepliesLoadedMsg{
		ThreadTS: "P1",
		Replies:  []messages.MessageItem{{TS: "R1"}, {TS: "R5"}},
	})

	if len(got) != 1 || got[0] != "C1/P1/R5" {
		t.Fatalf("Mark calls = %v, want one call marking up to the newest reply", got)
	}
	if cmd == nil {
		t.Fatal("Update returned no cmd; the mark cmd was dropped, so the read cursor would never be persisted")
	}
	var found bool
	for _, m := range drainBatch(cmd) {
		if _, ok := m.(markSentinelMsg); ok {
			found = true
		}
	}
	if !found {
		t.Fatal("the cmd returned by Mark did not reach the caller; ThreadMarkedLocalMsg would never reach the reducer")
	}
}

// TestThreadRepliesLoaded_BackfillsParentTimestampFromCache pins the
// threads-view open path: the parent is built from the list summary,
// which has text but no formatted time, so the reducer copies only the
// Timestamp from the cached thread and keeps the rest of the row.
func TestThreadRepliesLoaded_BackfillsParentTimestampFromCache(t *testing.T) {
	const threadTS = "1700000000.000100"
	cachedParent := messages.MessageItem{TS: threadTS, UserID: "U1", Text: "cached text", Timestamp: "9:01 AM"}
	reply := messages.MessageItem{TS: "1700000050.000200", UserID: "U2", Text: "reply", ThreadTS: threadTS}

	app := newTestApp(t,
		withThreadsView([]core.ThreadSummary{{
			ChannelID: "C1", ThreadTS: threadTS, ParentTS: threadTS, ParentUserID: "U1", ParentText: "summary text",
		}}),
		withView(ViewThreads),
	)
	app.SetThreadService(core.NewThreadService(core.ThreadServiceFuncs{
		CacheRead: func(ids.ChannelID, ids.ThreadTS) []messages.MessageItem {
			return []messages.MessageItem{cachedParent, reply}
		},
	}))
	app.focusedPanel = PanelMessages
	app.Update(keyCode(tea.KeyEnter))
	if !app.threadVisible || app.threadPanel.ParentMsg().Timestamp != "" {
		t.Fatalf("precondition: want an open thread whose summary-built parent has no time (visible %v, ts %q)",
			app.threadVisible, app.threadPanel.ParentMsg().Timestamp)
	}

	app.Update(ThreadRepliesLoadedMsg{ThreadTS: threadTS, Replies: []messages.MessageItem{reply}})

	if got := app.threadPanel.ParentMsg().Timestamp; got != cachedParent.Timestamp {
		t.Errorf("parent timestamp = %q, want %q backfilled from cache", got, cachedParent.Timestamp)
	}
	if got := app.threadPanel.ParentMsg().Text; got != "summary text" {
		t.Errorf("parent text = %q, want the summary's %q kept", got, "summary text")
	}
}
