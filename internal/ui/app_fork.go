package ui

import (
	"time"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/messages"
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
