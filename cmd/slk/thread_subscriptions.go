package main

import (
	"context"
	"encoding/json"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/debuglog"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
)

// threadsBadge is a workspace's followed-threads state as Slack last
// reported it: from client.counts at boot and on reconnect, from each
// update_global_thread_state event, and from each Threads view page.
// The sidebar's Threads badge and the workspace rail are drawn from it,
// as the Slack app draws them; slk does not count threads itself.
type threadsBadge struct {
	// hasUnreads is the rail's thread half (railUnreadWorkspaces),
	// read on the UI goroutine and written from socket and network
	// goroutines.
	hasUnreads atomic.Bool
	// events stamps each update_global_thread_state, so the reconnect
	// catch-up can drop a client.counts snapshot an event superseded
	// mid-request (see badgeEventClock).
	events badgeEventClock
}

// apply records state and hands its count to the UI. Every reader the
// UI's ThreadsBadgeUpdatedMsg arm recomputes (the rail) sees the new
// value, because it is stored before the message is sent.
func (b *threadsBadge) apply(teamID string, state slackclient.ThreadsAggregate, send func(tea.Msg)) {
	b.hasUnreads.Store(state.HasUnreads)
	send(ui.ThreadsBadgeUpdatedMsg{TeamID: teamID, UnreadThreads: state.UnreadThreads})
}

// threadsViewer is the one call the Threads view makes:
// subscriptions.thread.getView, one page at a time.
type threadsViewer interface {
	GetThreadsView(ctx context.Context, cursor string) (slackclient.ThreadsViewPage, error)
}

// fetchThreadsPage loads one page of teamID's Threads view (the first
// when cursor is "") and builds the view's rows from it. Each row's
// read cursor is written to thread_subscriptions, where the thread
// panel reads its "── new ──" boundary. onState receives the page's
// threads_state, the same badge client.counts reports.
//
// A failed fetch returns the error and no rows: the view keeps what it
// shows and draws its banner.
func fetchThreadsPage(ctx context.Context, client threadsViewer, db *cache.DB, teamID, cursor string, onState func(slackclient.ThreadsAggregate)) ui.ThreadsListLoadedMsg {
	msg := ui.ThreadsListLoadedMsg{TeamID: teamID, Cursor: cursor}
	page, err := client.GetThreadsView(ctx, cursor)
	if err != nil {
		debuglog.Backfill("team=%s threads-view cursor=%q err=%v", teamID, cursor, err)
		msg.Err = err
		return msg
	}
	msg.NextCursor = page.NextCursor
	onState(page.State)
	msg.Summaries = make([]core.ThreadSummary, 0, len(page.Threads))
	for _, it := range page.Threads {
		root := it.Root
		s := core.ThreadSummary{
			ChannelID:    root.Channel,
			ThreadTS:     root.ThreadTimestamp,
			ParentUserID: root.User,
			ParentText:   root.Text,
			ParentTS:     root.Timestamp,
			ReplyCount:   root.ReplyCount,
			LastReplyTS:  root.LatestReply,
			LastReplyBy:  it.LastReplyBy,
			// Slack's own count of unread threads is the number whose
			// newest reply is past the read cursor.
			Unread: root.LatestReply > root.LastRead,
		}
		if s.ParentUserID == "" {
			s.ParentUserID = root.BotID
		}
		if ch, err := db.GetChannel(root.Channel); err == nil {
			s.ChannelName, s.ChannelType = ch.Name, ch.Type
		}
		msg.Summaries = append(msg.Summaries, s)
		recordThreadCursor(db, teamID, root.Channel, root)
	}
	if debuglog.Enabled() {
		unread := 0
		for _, s := range msg.Summaries {
			if s.Unread {
				unread++
			}
		}
		debuglog.Backfill("team=%s threads-view cursor=%q rows=%d unread=%d next=%q badge=%d",
			teamID, cursor, len(msg.Summaries), unread, page.NextCursor, page.State.UnreadThreads)
	}
	return msg
}

// recordThreadCursor writes a followed thread's read cursor, as a
// getView page or a conversations.replies fetch reports it on the
// thread's root, to thread_subscriptions. A root that is not
// subscribed, or carries no cursor, writes nothing.
func recordThreadCursor(db *cache.DB, teamID, channelID string, root slack.Message) {
	if !root.Subscribed || root.LastRead == "" {
		return
	}
	threadTS := root.ThreadTimestamp
	if threadTS == "" {
		threadTS = root.Timestamp
	}
	if err := db.UpdateThreadLastRead(teamID, channelID, threadTS, root.LastRead); err != nil {
		debuglog.Cache("recordThreadCursor: %s/%s: %v", channelID, threadTS, err)
	}
}

// threadLastRead returns the user's read cursor in a thread for the
// thread panel's "── new ──" boundary: the cached cursor when there is
// one, else the last_read Slack put on the thread's cached root
// message. "" when neither is known.
func threadLastRead(db *cache.DB, teamID, channelID, threadTS string) string {
	lastRead, err := db.GetThreadLastRead(teamID, channelID, threadTS)
	if err != nil {
		debuglog.Cache("threadLastRead: %s/%s: %v", channelID, threadTS, err)
	}
	if lastRead != "" {
		return lastRead
	}
	root, err := db.GetMessage(channelID, threadTS)
	if err != nil || root.RawJSON == "" {
		return ""
	}
	var cursor struct {
		LastRead string `json:"last_read"`
	}
	_ = json.Unmarshal([]byte(root.RawJSON), &cursor)
	return cursor.LastRead
}

// OnGlobalThreadState applies Slack's push of the workspace's
// followed-threads badge, and asks the Threads view, if it is open on
// this workspace, to reload its first page: the change is a reply in a
// followed thread or a read somewhere.
func (h *rtmEventHandler) OnGlobalThreadState(state slackclient.ThreadsAggregate) {
	if h.wsCtx == nil || h.program == nil {
		return
	}
	h.wsCtx.ThreadsBadge.events.stamp()
	h.wsCtx.ThreadsBadge.apply(h.workspaceID, state, h.program.Send)
	if h.isActive != nil && !h.isActive() {
		return
	}
	h.program.Send(ui.ThreadsListDirtyMsg{TeamID: h.workspaceID})
}

// OnThreadSubscriptionChanged persists a subscribe/unsubscribe event
// in the thread_subscriptions table, and asks the Threads view, if it
// is open on this workspace, to reload its first page so a new
// subscription shows up or an unsubscribed thread goes away.
func (h *rtmEventHandler) OnThreadSubscriptionChanged(channelID, threadTS, lastRead string, active bool) {
	// Persist regardless of active-workspace state, matching
	// OnChannelMarked and OnMessage: the cursor is what the thread
	// panel reads when the thread is opened, whichever workspace is on
	// screen now.
	if h.db != nil {
		if err := h.db.UpsertThreadSubscription(h.workspaceID, channelID, threadTS, lastRead, active); err != nil {
			debuglog.Cache("OnThreadSubscriptionChanged: UpsertThreadSubscription %s/%s: %v",
				channelID, threadTS, err)
		}
	}
	// The Threads view reloads only on screen, which an inactive
	// workspace's is not.
	if h.isActive != nil && !h.isActive() {
		return
	}
	if h.program != nil {
		h.program.Send(ui.ThreadsListDirtyMsg{TeamID: h.workspaceID})
	}
}
