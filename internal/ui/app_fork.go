package ui

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
)

// appFork is the fork's App state, embedded in App so its fields read
// as App's own.
type appFork struct {
	// threadFullscreen zooms the thread over the whole messages
	// region (the `t` toggle). Only meaningful while threadVisible.
	// CloseThread resets it so a reopened thread starts unzoomed, and
	// threads-view activation lifts it (the list it navigates to
	// renders in the covered region).
	threadFullscreen bool

	// provisionalTeamID/Name: cache-first boot state — see cachefirst_fork.go.
	provisionalTeamID   string
	provisionalTeamName string
	// readStateReader: App-side copy for applyCachedLastRead (cachefirst_fork.go).
	readStateReader func() map[string]core.ReadState

	agentSidebar agentSidebar

	// herdrTabViewed is the pane's viewed state from the herdr focus
	// watcher; herdrTabViewKnown is false when nothing reports it (slk
	// running outside herdr). Read through PaneViewed, never directly:
	// the pair only means anything together.
	herdrTabViewed    bool
	herdrTabViewKnown bool

	lastView viewMemo

	cacheStaleBefore map[string]time.Time

	// paneStateRecorder persists the pane's open channel/thread for
	// restore-on-relaunch; nil when the restore feature is disabled.
	// lastPaneReport dedupes: setThreadPanel re-fires on every replies
	// reload of the same thread.
	paneStateRecorder PaneStateRecorder
	lastPaneReport    paneReport

	// asyncPaste routes paste off the UI goroutine for a slow (bridged)
	// clipboard reader; see paste_async_fork.go.
	asyncPaste asyncPasteState

	pasteThumbFocus pasteThumbFocus   // compose_preview_focus_fork.go
	appShortcuts    appShortcutsState // appshortcuts.go
	appHome         appHomeState      // apphome.go
	profileCard     profileCardState  // profilecard.go
	ephemerals      ephemerals        // ephemeral_fork.go

	// pickerInHerdr marks a links picker opened by the O or alt+o
	// keybinding: Enter then dispatches OpenLinkMsg with InHerdr set to it.
	pickerInHerdr HerdrTarget

	// pickerCopyables backs the "copy" pickerKind (copy_from_message.go).
	pickerCopyables []messages.Copyable

	// linkPreviewGen guards the picker's async permalink previews:
	// bumped on every links-picker open, echoed by LinkPreviewMsg.Gen,
	// so previews from an earlier picker can't fill a later one's rows.
	linkPreviewGen uint64
	// linkPreviewAsked holds the picker rows whose preview has been
	// fetched for this linkPreviewGen, see linkPreviewsInView.
	linkPreviewAsked map[int]bool

	// reloader forces every workspace's websocket to reconnect
	// (ctrl+r / :reload). Nil until SetReloader.
	reloader ReloadFunc
	// reconnectTickerOn guards the reconnect-countdown tick chain
	// (reducer_io.go) against parallel chains when several
	// reconnect-wait messages arrive in a row.
	reconnectTickerOn bool
	// connStates caches the latest per-workspace connection state so
	// the statusbar segment can follow the active workspace across
	// switches (mirrors presenceController's per-team status cache).
	// Nil until the first ConnectionStateMsg; see storeConnState.
	connStates map[string]connectionState

	// startupLinkNav is a permalink navigation requested on the command
	// line (slk <link>). Set via SetStartupLink before the program
	// starts; consumed once by reduceWorkspaceReady, which promotes it
	// to pendingLinkNav in place of the last-visited channel restore.
	startupLinkNav *pendingLinkNav

	// herdrTabOpener opens a Slack permalink in a new herdr tab running
	// a second slk instance (the O keybinding). Blocking; routeLink
	// calls it inside a tea.Cmd. Nil outside a herdr pane, which makes
	// O route exactly like o. See SetHerdrTabOpener.
	herdrTabOpener func(url, label string, focus bool) error
	// herdrPaneOpener is herdrTabOpener's alt+o twin: a new herdr pane
	// to the right of this one. See SetHerdrPaneOpener.
	herdrPaneOpener func(url, splitPaneID string, focus bool) (string, error)

	// sixelMemo lets an unchanged frame reuse its published frame ID
	// (see sixelpaint_fork.go).
	sixelMemo sixelFrameMemo
}

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

// keepFocusOffHiddenMessages runs after every Update: a zoomed thread
// covers the messages pane, and plenty of handlers park focus there
// without knowing about the zoom (window split, hiding a focused
// sidebar). Normalizing here rather than at each of those keeps the
// invariant in one place: focus is never on a pane the frame won't
// draw.
func (a *App) keepFocusOffHiddenMessages() {
	if a.threadVisible && a.threadFullscreen && a.focusedPanel == PanelMessages {
		a.focusedPanel = PanelThread
	}
}

// permalinkRowText is a permalink row's fallback text: what the picker
// shows until (or instead of, on fetch failure) the message preview,
// "Today · thread reply". Where the link points is the row's Side
// column, permalinkPlace.
func permalinkRowText(pl slackurl.Permalink) string {
	var parts []string
	if date := messages.DateFromTS(string(pl.MessageTS)); date != "" {
		parts = append(parts, messages.FormatDateSeparator(date))
	}
	if pl.ThreadTS != "" && string(pl.ThreadTS) != string(pl.MessageTS) {
		parts = append(parts, "thread reply")
	}
	return strings.Join(parts, " · ")
}

// permalinkPlace is where a permalink points: "#channel" for an in-app
// link, "sub.slack.com" for a foreign workspace.
func (a *App) permalinkPlace(pl slackurl.Permalink, inApp bool) string {
	if !inApp {
		return pl.Subdomain + ".slack.com"
	}
	// inApp implies the Lookup succeeds (linkOpensInApp requires it).
	name, chType, _ := a.channels.Lookup(pl.ChannelID)
	return channelDisplayName(name, chType)
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
		sender, text, err := messageSvc.Preview(ctx, pl.ChannelID, pl.MessageTS, pl.ThreadTS)
		if err != nil {
			debuglog.General("linkPreview: %s/%s: %v", pl.ChannelID, pl.MessageTS, err)
			return nil
		}
		if text == "" {
			return nil
		}
		return LinkPreviewMsg{Index: index, Gen: gen, ChannelID: string(pl.ChannelID), Sender: sender, Text: text}
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

// threadRefetchOnReconnect returns the open-thread refetch cmd when a
// workspace's connection state transitions back to Connected, or nil.
// Must run before the caller stores the new state in connStates: the
// transition is detected against the previously cached state. A first
// connect (no cached entry) doesn't refetch — the boot thread-open
// path fetches on its own — and only the active workspace qualifies,
// since the thread panel shows a thread of the active workspace.
func (a *App) threadRefetchOnReconnect(teamID string, newState statusbar.ConnectionState) tea.Cmd {
	if newState != statusbar.StateConnected || teamID == "" || teamID != a.activeTeamID {
		return nil
	}
	prev, ok := a.connStates[teamID]
	if !ok || prev.state == statusbar.StateConnected {
		return nil
	}
	return a.refetchOpenThreadCmd()
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

// threadMarkedRemoteFork is the fork's half of the ThreadMarkedRemoteMsg
// arm; true means the message is fully handled.
//
// The agent thread row is served here for every workspace, from m.Read:
// it has no threads-list summary to derive read state from. A mark from
// a background workspace (see NewMessageMsg.TeamID) goes no further.
//
// A read mark never moves the open panel's "── new ──" landmark, whoever
// issued it: marks land while the user is watching the thread, and
// yanking the landmark mid-read would hide which replies were new. It
// takes the self-echo route — list state only — and consumes any
// self-mark record so a stale one can't shield a later unread mark.
// Unread marks fall through to upstream's applyThreadMarkEcho.
func (a *App) threadMarkedRemoteFork(m ThreadMarkedRemoteMsg) bool {
	teamID := m.TeamID
	if teamID == "" {
		teamID = a.activeTeamID
	}
	if m.Read {
		a.markAgentThreadRead(teamID, m.ChannelID, m.ThreadTS)
	} else {
		a.markAgentThreadUnread(teamID, m.ChannelID, m.ThreadTS, m.LastRead)
	}
	if teamID != a.activeTeamID {
		return true
	}
	if !m.Read {
		return false
	}
	a.selfThreadMarks.consume(selfMarkKey{channelID: m.ChannelID, threadTS: m.ThreadTS, ts: m.LastRead})
	a.applyThreadMarkListState(m.ChannelID, m.ThreadTS, m.LastRead)
	return true
}
