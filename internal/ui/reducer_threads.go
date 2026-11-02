// internal/ui/reducer_threads.go
//
// Thread-family reducer for App.Update (Phase 4h).
//
// Owns the twelve Update arms that drive the thread panel, the
// threads-list view, and the thread-reply send path:
//
//	ThreadMarkedRemoteMsg       - apply an inbound thread_marked event
//	                              to the local read state, skipping the
//	                              panel landmark when it is the echo of
//	                              a mark slk itself issued.
//	ThreadMarkedLocalMsg        - outcome of an slk-initiated
//	                              subscriptions.thread.mark: apply the
//	                              accepted cursor to list state only,
//	                              leaving the open panel's landmark
//	                              where opening the thread put it, or
//	                              log and leave read state alone when
//	                              Slack rejected it. The
//	                              echo-suppression record is written at
//	                              each issue site before the mark goes
//	                              out, never here — see selfMarkDedup.
//	threadFetchDebounceMsg      - debounced j/k stop: fire the actual
//	                              thread fetch (drops stale generations
//	                              and post-navigation ticks).
//	ThreadRepliesLoadedMsg      - replies fetch returned: refresh the
//	                              panel, mark the thread as read, and
//	                              refresh the sidebar badge.
//	ThreadsViewActivatedMsg     - user opened the threads-list view:
//	                              switch view + focus, fetch its first
//	                              page from Slack, open the highlighted
//	                              thread of the page last loaded.
//	ThreadsListLoadedMsg        - a page landed: install the first page
//	                              or append a later one; never opens a
//	                              thread.
//	ThreadsBadgeUpdatedMsg      - Slack's count of unread followed
//	                              threads: store it, show it on the
//	                              sidebar's Threads row, refresh the rail.
//	ThreadsListDirtyMsg         - "the first page might be stale" from
//	                              any of its uncoordinated senders: open
//	                              a coalescing window, or join the open
//	                              one, while the view is on screen.
//	threadsListFetchMsg         - that window closing: refetch the first
//	                              page.
//	SendThreadReplyMsg          - user sent a reply: optimistic
//	                              placeholder + chat.postMessage call.
//	ThreadReplySentMsg          - reply landed: swap placeholder for
//	                              authoritative message, bump parent
//	                              reply count and the thread's row.
//	ThreadReplySendFailedMsg    - reply failed: roll back the
//	                              placeholder + fire SendFailed toast.
//
// Free reducer (not controller-absorbed): these arms cooperate on
// the thread panel, the threads-list view, the sidebar's threads
// badge, the active channel's messages pane, the self-send dedup,
// and the threads service. No single existing controller owns all
// of that cross-section, and creating one would be a rename rather
// than an extraction (see Phase 3's WorkspaceService skip rationale).
//
// The helpers (applyThreadMarkEcho, applyThreadMarkListState,
// openSelectedThreadCmd, loadMoreThreadsCmd) stay on App; this reducer
// calls them via `a`.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slack/mrkdwn"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
)

var reduceThreads reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case ThreadMarkedRemoteMsg:
		if a.threadMarkedRemoteFork(m) {
			return nil, true
		}
		// Via the echo helper: this event is also how slk's own thread
		// marks come back. See applyThreadMarkEcho.
		a.applyThreadMarkEcho(m.ChannelID, m.ThreadTS, m.LastRead)
		return nil, true

	case ThreadMarkedLocalMsg:
		if m.Err != nil {
			debuglog.Cache("ThreadMarkedLocalMsg: channel=%s thread_ts=%s failed: %v",
				m.ChannelID, m.ThreadTS, m.Err)
			return nil, true
		}
		// No self-mark recording here: both issue sites record before
		// handing off the cmd, so by the time this arrives the record
		// already exists and recording again would double-count and
		// suppress a second, foreign echo. See selfMarkDedup.
		//
		// List state only: this is slk's own mark completing, so the
		// open panel's landmark stays where opening the thread put it.
		// See applyThreadMarkListState.
		a.applyThreadMarkListState(m.ChannelID, m.ThreadTS, m.TS)
		a.markAgentThreadRead(a.activeTeamID, m.ChannelID, m.ThreadTS)
		return nil, true

	case threadFetchDebounceMsg:
		// Drop stale debounce ticks: a later j/k has scheduled a
		// fresh fetch and bumped the generation past this one.
		if m.gen != a.pendingThreadFetchGen {
			return nil, true
		}
		// Also drop if the user has navigated away (e.g. switched
		// to a different thread or closed the threads view) since
		// scheduling.
		if m.channelID != a.lastOpenedChannelID || m.threadTS != a.lastOpenedThreadTS {
			return nil, true
		}
		threads := a.threads
		chID := ids.ChannelID(m.channelID)
		threadTS := ids.ThreadTS(m.threadTS)
		var batch []tea.Cmd
		if cmd := cachedThreadRepliesCmd(threads, chID, threadTS); cmd != nil {
			batch = append(batch, cmd)
		}
		batch = append(batch, func() tea.Msg { return threads.Fetch(chID, threadTS) })
		return tea.Batch(batch...), true

	case ThreadRepliesLoadedMsg:
		if !(a.threadVisible && m.ThreadTS == a.threadPanel.ThreadTS()) {
			return nil, true
		}
		channelID := a.threadPanel.ChannelID()
		// nil Replies signals network failure (the fetcher logs the
		// error and returns nil); empty []MessageItem{} signals
		// "no replies yet". Skip the panel update on failure so a
		// transient blip doesn't blank a successfully-rendered
		// cached thread view. That cached view did not mark the
		// thread (see FromCache), so mark it now from what the user
		// is looking at; with no cached replies on screen there is
		// nothing they have seen to mark.
		if m.Replies == nil {
			if replies := a.threadPanel.Replies(); newestMarkableReplyTS(replies) != "" {
				return a.markFetchedThreadRead(channelID, m.ThreadTS, replies), true
			}
			return nil, true
		}
		parentMsg := a.threadPanel.ParentMsg()
		// Permalink-opened threads start with a stub parent (TS only),
		// and a thread opened from a reply row may carry the wrong row
		// as parent. The fetch that produced this msg also wrote the
		// full thread to cache — backfill the parent row from there.
		// Threads-view opens build the parent from the list summary,
		// which has text but no formatted time — take just that field.
		// setThreadPanel re-runs agent-thread detection, the first time
		// the root text is actually known on this path.
		if parentMsg.Text == "" || parentMsg.Timestamp == "" || parentMsg.TS != m.ThreadTS {
			if cached := a.threads.CacheRead(ids.ChannelID(channelID), ids.ThreadTS(m.ThreadTS)); len(cached) > 0 && cached[0].Text != "" {
				if parentMsg.Text == "" || parentMsg.TS != m.ThreadTS {
					parentMsg = cached[0]
				} else if parentMsg.Timestamp == "" {
					parentMsg.Timestamp = cached[0].Timestamp
				}
			}
		}
		labelCmd := a.setThreadPanel(parentMsg, m.Replies, channelID, m.ThreadTS)

		// Cached replies only render. Marking from them raced the
		// fetched result's mark with a cursor that may lag Slack:
		// the stale mark re-flagged the thread unread and, landing
		// last, persisted the stale cursor. See FromCache.
		if m.FromCache {
			return labelCmd, true
		}
		// A thread opened with no cursor known (never in a loaded
		// Threads page, no cached root) has one now: the fetch
		// recorded the root's, which predates the mark below.
		if a.threadPanel.LastReadTS() == "" {
			a.applyThreadUnreadBoundary(channelID, m.ThreadTS)
		}
		cmd := a.markFetchedThreadRead(channelID, m.ThreadTS, m.Replies)
		return tea.Batch(cmd, labelCmd), true

	case ThreadsViewActivatedMsg:
		_ = m
		a.view = ViewThreads
		a.sidebar.SetThreadsActive(true)
		// Activation navigates TO the threads list, which renders in
		// the region a zoomed thread covers. Lift the zoom so the
		// destination is on screen.
		a.threadFullscreen = false
		a.focusedPanel = PanelMessages
		var batch []tea.Cmd
		if a.activeTeamID != "" {
			// The list loads from Slack on every open, as in the Slack
			// app; the page last loaded this session stays on screen
			// until the new one lands.
			a.threadsRefreshKeepsOlder = false
			if len(a.threadsView.Summaries()) == 0 {
				a.threadsView.SetLoading(true)
			}
			threads := a.threads
			team := ids.TeamID(a.activeTeamID)
			batch = append(batch, func() tea.Msg { return threads.ListFetch(team, "") })
		}
		// Activation is a single event -- fire the fetch immediately
		// so the right thread panel populates without artificial delay.
		if cmd := a.openSelectedThreadCmd(false); cmd != nil {
			batch = append(batch, cmd)
		}
		if len(batch) == 0 {
			return nil, true
		}
		return tea.Batch(batch...), true

	case ThreadsListLoadedMsg:
		if m.TeamID != a.activeTeamID {
			return nil, true
		}
		a.threadsView.SetSubscriptionsAvailable(m.Err == nil)
		if m.Err != nil {
			a.threadsView.PageFailed(m.Cursor)
			return nil, true
		}
		if m.Cursor == "" {
			a.threadsView.SetFirstPage(m.Summaries, m.NextCursor, a.threadsRefreshKeepsOlder)
			a.threadsRefreshKeepsOlder = false
		} else {
			a.threadsView.AppendPage(m.Cursor, m.Summaries, m.NextCursor)
		}
		if a.view != ViewThreads {
			return nil, true
		}
		// A page never opens a thread: opening one marks it read on
		// Slack, which only the user may ask for. A refresh can also
		// move the selection (a thread just read leaves the first
		// page, as Slack lists unread threads first), and reopening
		// what is highlighted then would read the next thread, and
		// the next. A page that leaves the end of the list on screen
		// asks for the next one, as scrolling there would.
		return a.loadMoreThreadsCmd(), true

	case ThreadsBadgeUpdatedMsg:
		if a.threadsBadgeCounts == nil {
			a.threadsBadgeCounts = make(map[string]int)
		}
		a.threadsBadgeCounts[m.TeamID] = m.UnreadThreads
		if m.TeamID == a.activeTeamID {
			a.sidebar.SetThreadsUnreadCount(m.UnreadThreads)
		}
		// The rail's thread half is the same Slack state, stored
		// before this was sent.
		a.notifyReadStateChanged()
		return nil, true

	case ThreadsListDirtyMsg:
		// Only an open view refreshes: a closed one loads afresh when
		// it opens, and another workspace's is not on screen.
		if m.TeamID != a.activeTeamID || a.view != ViewThreads {
			return nil, true
		}
		// Drop it if a refresh is already waiting: that fetch has not
		// run yet, so it will see this change too.
		if a.threadsListFetchScheduled {
			return nil, true
		}
		a.threadsListFetchScheduled = true
		team := a.activeTeamID
		d := a.threadsDirtyDebounce
		if d == 0 {
			d = defaultThreadsDirtyDebounce
		}
		return tea.Tick(d, func(time.Time) tea.Msg {
			return threadsListFetchMsg{teamID: team}
		}), true

	case threadsListFetchMsg:
		// Cleared before either return so the window always reopens.
		// Leaving it set when the checks below drop the fetch would
		// freeze the threads list for the rest of the session.
		a.threadsListFetchScheduled = false
		if m.teamID != a.activeTeamID || a.view != ViewThreads {
			return nil, true
		}
		a.threadsRefreshKeepsOlder = true
		threads := a.threads
		team := ids.TeamID(a.activeTeamID)
		return func() tea.Msg { return threads.ListFetch(team, "") }, true

	case SendThreadReplyMsg:
		a.selfSend.BeginSend(m.ChannelID)
		a.recordEmojiInText(m.Text)
		// Instant-display: append an optimistic placeholder to the
		// thread panel immediately, before the chat.postMessage HTTP
		// round-trip. Mirrors the SendMessageMsg path; see there for
		// the LocalTS / swap-or-remove contract and the
		// mrkdwn.Convert rationale.
		localTS := a.selfSend.NextLocalTS()
		optimisticText, _ := mrkdwn.Convert(m.Text)
		if a.threadVisible && m.ThreadTS == a.threadPanel.ThreadTS() && m.ChannelID == a.threadPanel.ChannelID() {
			a.threadPanel.AddReply(messages.MessageItem{
				TS:        localTS,
				UserID:    a.currentUserID,
				UserName:  a.userNameFor(a.currentUserID),
				Text:      optimisticText,
				Timestamp: a.nowFormatted(),
				ThreadTS:  m.ThreadTS,
			})
		}
		// Broadcast ("Also send to #channel"): the reply will also
		// land in the parent channel feed as a thread_broadcast row.
		// The self-send dedup (selfSend.RecordSent in
		// ThreadReplySentMsg) suppresses the WS echo, so the only
		// way the feed shows the row promptly is this optimistic
		// add + the swap in ThreadReplySentMsg below.
		if m.Broadcast {
			for _, mm := range a.modelsForChannel(m.ChannelID) {
				mm.AppendMessage(messages.MessageItem{
					TS:        localTS,
					UserID:    a.currentUserID,
					UserName:  a.userNameFor(a.currentUserID),
					Text:      optimisticText,
					Timestamp: a.nowFormatted(),
					ThreadTS:  m.ThreadTS,
					Subtype:   "thread_broadcast",
				})
			}
		}
		threads := a.threads
		chID := ids.ChannelID(m.ChannelID)
		ts := ids.ThreadTS(m.ThreadTS)
		text := m.Text
		broadcast := m.Broadcast
		return func() tea.Msg {
			result := threads.SendReply(chID, ts, text, broadcast)
			switch r := result.(type) {
			case ThreadReplySentMsg:
				r.LocalTS = localTS
				return r
			case ThreadReplySendFailedMsg:
				r.LocalTS = localTS
				r.Broadcast = broadcast
				return r
			}
			return result
		}, true

	case ThreadReplySentMsg:
		// chat.postMessage for the thread reply landed. If a
		// "local:..." placeholder is in the thread panel from the
		// instant-display path (SendThreadReplyMsg above), swap it
		// for the authoritative message; otherwise fall back to
		// UpsertSelfSentReply.
		//
		// Note: the internal Slack flannel WebSocket does not always
		// echo self-posted thread replies as a plain "message" event,
		// so we cannot rely on the WS echo alone -- the HTTP response
		// must apply all the side effects (parent reply count, threads
		// dirty) here.
		a.selfSend.EndSend(m.ChannelID)
		if m.Message.TS == "" {
			return nil, true
		}
		a.selfSend.RecordSent(m.Message.TS)
		// Update the thread panel whenever the visible thread
		// matches, regardless of activeChannelID. When a thread is
		// opened from the threads view, activeChannelID is not
		// switched to the thread's channel, so gating on it here
		// meant the user's own reply was sent to Slack but never
		// appended locally -- they had to leave and re-enter the
		// thread to see it.
		if a.threadVisible && m.ThreadTS == a.threadPanel.ThreadTS() && m.ChannelID == a.threadPanel.ChannelID() {
			if !a.threadPanel.SwapLocalSentReply(m.LocalTS, m.Message) {
				a.threadPanel.UpsertSelfSentReply(m.Message)
			}
		}
		// Broadcast: also land the authoritative message in the
		// channel feed, swapping the optimistic thread_broadcast
		// placeholder added in SendThreadReplyMsg. Mirrors
		// MessageSentMsg's swap-or-upsert contract.
		if m.Broadcast {
			for _, mm := range a.modelsForChannel(m.ChannelID) {
				item := cloneMessageItem(m.Message)
				if !mm.SwapLocalSent(m.LocalTS, item) {
					mm.UpsertSelfSent(item)
				}
			}
		}
		for _, mm := range a.modelsForChannel(m.ChannelID) {
			mm.IncrementReplyCount(m.ThreadTS, m.Message.TS)
		}
		a.applyThreadsReply(m.ChannelID, m.Message, true)
		return nil, true

	case ThreadReplySendFailedMsg:
		// chat.postMessage for the thread reply failed; roll back
		// the optimistic placeholder. Mirrors MessageSendFailedMsg.
		// For broadcasts, also roll back the thread_broadcast row
		// optimistically added to the channel feed.
		a.selfSend.EndSend(m.ChannelID)
		if a.threadVisible && m.ThreadTS == a.threadPanel.ThreadTS() && m.ChannelID == a.threadPanel.ChannelID() && m.LocalTS != "" {
			a.threadPanel.RemoveLocalSentReply(m.LocalTS)
		}
		if m.Broadcast && m.LocalTS != "" {
			for _, mm := range a.modelsForChannel(m.ChannelID) {
				mm.RemoveLocalSent(m.LocalTS)
			}
		}
		if cmd := a.sendFailedCmd(m.ChannelID, m.Reason); cmd != nil {
			return cmd, true
		}
		reason := m.Reason
		return func() tea.Msg {
			return statusbar.SendFailedMsg{Reason: reason}
		}, true
	}
	return nil, false
}

// newestMarkableReplyTS returns the ts of the newest reply a read
// cursor may land on, or "" when there is none. Ephemeral replies are
// skipped: Slack never returns them from conversations.replies, so a
// cursor on one would sit behind a message Slack does not know.
func newestMarkableReplyTS(replies []messages.MessageItem) string {
	for i := len(replies) - 1; i >= 0; i-- {
		if !replies[i].Ephemeral && replies[i].TS != "" {
			return replies[i].TS
		}
	}
	return ""
}

// markOpenThreadRead marks the open thread read up to the newest of
// replies, the replies the user has now seen. Server-side:
// subscriptions.thread.mark with the latest reply ts (or the parent ts
// when the thread has no replies); the returned cmd persists the cursor
// and reports back as ThreadMarkedLocalMsg. Local-side: clear the Unread
// flag in the threads-list view and refresh the sidebar's threads-row
// badge so the UI reflects the change immediately, regardless of which
// path (messages pane or threads view) opened the thread.
func (a *App) markOpenThreadRead(channelID, threadTS string, replies []messages.MessageItem) tea.Cmd {
	latestTS := threadTS
	if t := newestMarkableReplyTS(replies); t != "" {
		latestTS = t
	}
	var cmd tea.Cmd
	if channelID != "" && threadTS != "" {
		cmd = teaCmd(a.threads.Mark(
			ids.ChannelID(channelID),
			ids.ThreadTS(threadTS),
			ids.MessageTS(latestTS),
		))
		if cmd != nil {
			// Record BEFORE the cmd runs, i.e. before the mark is
			// issued: Slack's thread_marked broadcast races the
			// mark's own HTTP response, and this is the mark whose
			// echo would otherwise wipe the landmark the user just
			// opened the thread to read. Mark only builds the cmd,
			// so nothing has been sent yet and this record cannot
			// lose that race. A nil cmd means no mark will be
			// issued (no active workspace), hence no echo to
			// suppress. See selfMarkDedup.
			a.selfThreadMarks.record(selfMarkKey{
				channelID: channelID, threadTS: threadTS, ts: latestTS,
			})
		}
	}
	// Optimistic local recompute so the row and badge update
	// immediately rather than waiting on the round-trip. MarkByThreadTSReadAt
	// sets Unread = summary.LastReplyTS > latestTS, so it usually
	// clears the flag; but it re-derives rather than clears, so it
	// can also set it. That happens when the summary knows a newer
	// reply than this fetch returned (LastReplyTS > latestTS), in
	// which case the thread really is still unread and stays
	// flagged. ThreadMarkedLocalMsg reconciles this against the
	// cursor Slack accepted on success; a rejected mark leaves
	// state untouched and heals on the next list refresh. The badge
	// moves now, before the mark is sent, so Slack's
	// update_global_thread_state for it can only land after.
	before := a.threadsView.UnreadCount()
	if a.threadsView.MarkByThreadTSReadAt(channelID, threadTS, latestTS) {
		a.adjustThreadsBadge(a.threadsView.UnreadCount() - before)
	}
	return cmd
}

// loadMoreThreadsCmd fetches the Threads view's next page when the user
// has come near the end of the loaded rows (threadsview.NextPage), or
// returns nil.
func (a *App) loadMoreThreadsCmd() tea.Cmd {
	if a.activeTeamID == "" {
		return nil
	}
	cursor, ok := a.threadsView.NextPage()
	if !ok {
		return nil
	}
	threads := a.threads
	team := ids.TeamID(a.activeTeamID)
	return func() tea.Msg { return threads.ListFetch(team, cursor) }
}
