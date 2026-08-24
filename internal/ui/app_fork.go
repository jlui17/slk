package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages"
)

// threadMarkDebounceDelay is how long scheduleThreadMark waits after a live
// reply lands in the open thread panel before marking the thread read. Long
// enough to coalesce an agent-thread reply burst into one mark, short enough
// that the cursor advances before the user could plausibly navigate away.
const threadMarkDebounceDelay = 500 * time.Millisecond

// focusPeerWhileZoomed is the whole focus cycle while the thread is
// zoomed: the messages pane isn't drawn, so it's Sidebar <-> Thread,
// or Thread alone when the sidebar is hidden too. A two-stop cycle is
// symmetric, so FocusNext and FocusPrev share it.
func (a *App) focusPeerWhileZoomed() Panel {
	if a.sidebarVisible && a.focusedPanel == PanelThread {
		return PanelSidebar
	}
	return PanelThread
}

// ToggleThreadFullscreen zooms the open thread over the whole messages
// region, or restores the side-by-side layout. No-op with no thread
// open. Zooming pulls focus off the messages pane, which stops being
// drawn.
func (a *App) ToggleThreadFullscreen() {
	if !a.threadVisible {
		return
	}
	a.clearSelections()
	a.threadFullscreen = !a.threadFullscreen
	if a.threadFullscreen && a.focusedPanel == PanelMessages {
		a.focusedPanel = PanelThread
	}
}

// scheduleThreadMark returns a tea.Cmd that fires a threadMarkDebounceMsg
// for the open thread panel's (channelID, threadTS) after the mark-debounce
// interval. Called once per live reply landing in the panel while the pane
// is viewed; the generation bump means only the burst's last tick survives,
// so a burst coalesces into a single subscriptions.thread.mark call.
func (a *App) scheduleThreadMark(channelID, threadTS string) tea.Cmd {
	a.pendingThreadMarkGen++
	gen := a.pendingThreadMarkGen
	return tea.Tick(threadMarkDebounceDelay, func(time.Time) tea.Msg {
		return threadMarkDebounceMsg{channelID: channelID, threadTS: threadTS, gen: gen}
	})
}

// latestRealReplyTS returns the newest reply TS in the open thread panel,
// skipping optimistic "local:" placeholders whose Slack TS isn't known yet.
// Returns "" when the panel holds no real replies — a mark is only ever
// scheduled after a real reply rendered, so an empty panel means it was
// cleared and is repopulating (close/reopen inside the debounce window);
// marking then would use a stale TS and regress the cursor.
func (a *App) latestRealReplyTS() string {
	replies := a.threadPanel.Replies()
	for i := len(replies) - 1; i >= 0; i-- {
		ts := replies[i].TS
		if ts != "" && !strings.HasPrefix(ts, "local:") {
			return ts
		}
	}
	return ""
}

// permalinkRowText is a permalink row's fallback display: what the
// picker shows until (or instead of, on fetch failure) the message
// preview. In-app links decode to "#channel · Today · thread reply";
// foreign-workspace links to "sub.slack.com · Today".
func (a *App) permalinkRowText(pl slackurl.Permalink, inApp bool) string {
	parts := []string{pl.Subdomain + ".slack.com"}
	if inApp {
		// inApp implies the Lookup succeeds (linkOpensInApp requires it).
		name, chType, _ := a.channels.Lookup(pl.ChannelID)
		parts[0] = channelDisplayName(name, chType)
	}
	if date := messages.DateFromTS(string(pl.MessageTS)); date != "" {
		parts = append(parts, messages.FormatDateSeparator(date))
	}
	if pl.ThreadTS != "" && string(pl.ThreadTS) != string(pl.MessageTS) {
		parts = append(parts, "thread reply")
	}
	return strings.Join(parts, " · ")
}

func channelDisplayName(name, chType string) string {
	return messages.ChannelGlyph(chType) + name
}

// fetchLinkPreview resolves a picker row's target message via
// MessageService.Preview off the Update loop. Failures and unresolved
// messages produce no msg: the row keeps its permalinkRowText.
func (a *App) fetchLinkPreview(gen uint64, index int, pl slackurl.Permalink) tea.Cmd {
	messageSvc := a.messageSvc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		userID, text, err := messageSvc.Preview(ctx, pl.ChannelID, pl.MessageTS, pl.ThreadTS)
		if err != nil {
			debuglog.General("linkPreview: %s/%s: %v", pl.ChannelID, pl.MessageTS, err)
			return nil
		}
		if text == "" {
			return nil
		}
		return LinkPreviewMsg{Index: index, Gen: gen, ChannelID: string(pl.ChannelID), UserID: userID, Text: text}
	}
}

// SetReloader sets the callback that forces every workspace's
// websocket to reconnect (ctrl+r / :reload).
func (a *App) SetReloader(fn ReloadFunc) {
	a.reloader = fn
}

// reloadConnections runs the manual reload and acknowledges it with a
// toast. No-op before SetReloader is wired.
func (a *App) reloadConnections() tea.Cmd {
	if a.reloader == nil {
		return nil
	}
	a.reloader()
	cmds := []tea.Cmd{toastWithClear(a, "Reloading connections…", 2*time.Second)}
	if refetch := a.refetchOpenThreadCmd(); refetch != nil {
		cmds = append(cmds, refetch)
	}
	return tea.Batch(cmds...)
}

// refetchOpenThreadCmd returns a cmd that refetches the open thread
// panel's replies from Slack, or nil when no thread is open. Reload
// needs this because a thread reply swallowed by a websocket gap is
// unrecoverable by reconnect catch-up alone: the channel-level backfill
// reads conversations.history, which never returns thread replies, and
// the thread catch-up writes only to the cache, which an open panel
// never re-reads.
func (a *App) refetchOpenThreadCmd() tea.Cmd {
	if !a.threadVisible || a.threadPanel.ThreadTS() == "" {
		return nil
	}
	threads := a.threads
	chID := ids.ChannelID(a.threadPanel.ChannelID())
	threadTS := ids.ThreadTS(a.threadPanel.ThreadTS())
	return func() tea.Msg { return threads.Fetch(chID, threadTS) }
}

// markThreadReadLocally clears one thread's local unread state everywhere
// it is shown: the threads-list row, the sidebar's threads badge, and the
// agent-sidebar row. Every path that decides a thread has been read
// funnels here so a new one can't update two of the three and leave the
// herdr row claiming unread replies the user has already seen.
func (a *App) markThreadReadLocally(channelID, threadTS string) {
	if a.threadsView.MarkByThreadTSRead(channelID, threadTS) {
		a.sidebar.SetThreadsUnreadCount(a.threadsView.UnreadCount())
	}
	a.markAgentThreadRead(a.activeTeamID, channelID, threadTS)
}
