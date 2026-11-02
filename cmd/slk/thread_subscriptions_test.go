package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
)

// fakeThreadsViewer serves one canned page and records the cursors it
// was asked for.
type fakeThreadsViewer struct {
	page    slackclient.ThreadsViewPage
	err     error
	cursors []string
}

func (f *fakeThreadsViewer) GetThreadsView(_ context.Context, cursor string) (slackclient.ThreadsViewPage, error) {
	f.cursors = append(f.cursors, cursor)
	return f.page, f.err
}

func rootMsg(channel, ts, lastRead, latestReply string, subscribed bool) slack.Message {
	var m slack.Message
	m.Channel, m.Timestamp, m.ThreadTimestamp = channel, ts, ts
	m.LastRead, m.LatestReply, m.Subscribed = lastRead, latestReply, subscribed
	return m
}

func TestFetchThreadsPage_BuildsRowsAndRecordsCursors(t *testing.T) {
	db := newTestDB(t)
	if err := db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "private"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	unread := rootMsg("C1", "1700000100.000000", "1700000150.000000", "1700000200.000000", true)
	unread.User, unread.Text, unread.ReplyCount = "U1", "parent", 4
	read := rootMsg("C2", "1700000000.000000", "1700000090.000000", "1700000090.000000", true)
	read.BotID = "B1"
	viewer := &fakeThreadsViewer{page: slackclient.ThreadsViewPage{
		Threads: []slackclient.ThreadsViewItem{
			{Root: unread, LastReplyBy: "U2"},
			{Root: read},
		},
		NextCursor: "1700000090.000000",
		State:      slackclient.ThreadsAggregate{HasUnreads: true, UnreadThreads: 7},
	}}
	var states []slackclient.ThreadsAggregate

	msg := fetchThreadsPage(context.Background(), viewer, db, "T1", "1700000300.000000",
		func(s slackclient.ThreadsAggregate) { states = append(states, s) })

	if !reflect.DeepEqual(viewer.cursors, []string{"1700000300.000000"}) {
		t.Errorf("cursors = %v, want the one asked for", viewer.cursors)
	}
	if msg.Err != nil || msg.TeamID != "T1" || msg.Cursor != "1700000300.000000" || msg.NextCursor != "1700000090.000000" {
		t.Errorf("msg = %+v", msg)
	}
	want := []core.ThreadSummary{
		{
			ChannelID: "C1", ChannelName: "general", ChannelType: "private",
			ThreadTS: "1700000100.000000", ParentUserID: "U1", ParentText: "parent", ParentTS: "1700000100.000000",
			ReplyCount: 4, LastReplyTS: "1700000200.000000", LastReplyBy: "U2", Unread: true,
		},
		{
			ChannelID: "C2", ThreadTS: "1700000000.000000", ParentUserID: "B1", ParentTS: "1700000000.000000",
			LastReplyTS: "1700000090.000000",
		},
	}
	if !reflect.DeepEqual(msg.Summaries, want) {
		t.Errorf("summaries =\n%+v\nwant\n%+v", msg.Summaries, want)
	}
	if !reflect.DeepEqual(states, []slackclient.ThreadsAggregate{{HasUnreads: true, UnreadThreads: 7}}) {
		t.Errorf("states = %+v, want the page's threads_state", states)
	}
	if got := mustThreadLastRead(t, db, "T1", "C1", "1700000100.000000"); got != "1700000150.000000" {
		t.Errorf("cached cursor = %q, want the root's last_read", got)
	}
}

// A failed page carries the error and no rows; the view keeps what it
// shows.
func TestFetchThreadsPage_Error(t *testing.T) {
	viewer := &fakeThreadsViewer{err: errors.New("ratelimited")}
	called := false
	msg := fetchThreadsPage(context.Background(), viewer, newTestDB(t), "T1", "",
		func(slackclient.ThreadsAggregate) { called = true })
	if msg.Err == nil || msg.Summaries != nil || called {
		t.Errorf("msg = %+v, onState called = %v; want the error alone", msg, called)
	}
}

// A replies root writes the cursor only for a thread the user follows.
func TestRecordThreadCursor(t *testing.T) {
	db := newTestDB(t)
	recordThreadCursor(db, "T1", "C1", rootMsg("", "1700000100.000000", "1700000150.000000", "", false))
	if got := mustThreadLastRead(t, db, "T1", "C1", "1700000100.000000"); got != "" {
		t.Errorf("an unfollowed root wrote cursor %q", got)
	}
	recordThreadCursor(db, "T1", "C1", rootMsg("", "1700000100.000000", "1700000150.000000", "", true))
	if got := mustThreadLastRead(t, db, "T1", "C1", "1700000100.000000"); got != "1700000150.000000" {
		t.Errorf("cursor = %q, want the followed root's last_read", got)
	}
}

// The thread panel's boundary comes from the cached cursor, else from
// the last_read Slack put on the cached root.
func TestThreadLastRead_FallsBackToTheCachedRoot(t *testing.T) {
	db := newTestDB(t)
	if got := threadLastRead(db, "T1", "C1", "1700000100.000000"); got != "" {
		t.Errorf("nothing cached: got %q", got)
	}
	if err := db.UpsertMessage(cache.Message{
		TS: "1700000100.000000", ChannelID: "C1", WorkspaceID: "T1", ThreadTS: "1700000100.000000",
		RawJSON: `{"ts":"1700000100.000000","last_read":"1700000120.000000","subscribed":true}`,
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}
	if got := threadLastRead(db, "T1", "C1", "1700000100.000000"); got != "1700000120.000000" {
		t.Errorf("root only: got %q, want its last_read", got)
	}
	if err := db.UpdateThreadLastRead("T1", "C1", "1700000100.000000", "1700000180.000000"); err != nil {
		t.Fatalf("UpdateThreadLastRead: %v", err)
	}
	if got := threadLastRead(db, "T1", "C1", "1700000100.000000"); got != "1700000180.000000" {
		t.Errorf("cursor cached: got %q, want the cached cursor over the root", got)
	}
}

// update_global_thread_state sets the badge and the rail's state for
// any workspace, and reloads the Threads view only for the one on
// screen.
func TestOnGlobalThreadState(t *testing.T) {
	for _, active := range []bool{true, false} {
		sender := &captureSender{}
		wctx := &WorkspaceContext{}
		h := &rtmEventHandler{
			wsCtx:       wctx,
			workspaceID: "T1",
			isActive:    func() bool { return active },
			program:     sender,
		}
		h.OnGlobalThreadState(slackclient.ThreadsAggregate{HasUnreads: true, UnreadThreads: 3})

		want := []tea.Msg{ui.ThreadsBadgeUpdatedMsg{TeamID: "T1", UnreadThreads: 3}}
		if active {
			want = append(want, ui.ThreadsListDirtyMsg{TeamID: "T1"})
		}
		if !reflect.DeepEqual(sender.sent, want) {
			t.Errorf("active=%v: sent %+v, want %+v", active, sender.sent, want)
		}
		if !wctx.ThreadsBadge.hasUnreads.Load() || wctx.ThreadsBadge.events.current() == 0 {
			t.Errorf("active=%v: the rail's thread state or the event clock was not updated", active)
		}
	}
}
