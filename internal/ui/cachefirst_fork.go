// internal/ui/cachefirst_fork.go
//
// Cache-first boot paint. main.go sends one WorkspaceCachedMsg per run
// — for the workspace that would claim the initial active slot — built
// purely from sqlite, before any network call. The reducer here paints
// the sidebar and selects the restored channel so its cached messages
// render through the existing tier dispatch; the authoritative
// WorkspaceReadyMsg later replaces everything via
// reconcileProvisionalSelection.
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/gammons/slk/internal/ui/styles"
)

// WorkspaceCachedMsg is the provisional, cache-only twin of
// WorkspaceReadyMsg. LastChannelID must be the same value the real
// path will carry (restoredChannelFor over the same db state), so the
// provisional selection and the restored one cannot disagree.
type WorkspaceCachedMsg struct {
	TeamID        string
	TeamName      string
	Theme         string
	SidebarWidth  int
	Channels      []sidebar.ChannelItem
	FinderItems   []channelfinder.Item
	LastChannelID string
}

var reduceWorkspaceCached reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(WorkspaceCachedMsg)
	if !ok {
		return nil, false
	}
	return reduceWorkspaceCachedMsg(a, m), true
}

func reduceWorkspaceCachedMsg(a *App, m WorkspaceCachedMsg) tea.Cmd {
	if a.activeTeamID != "" || a.bootstrap.initialActiveClaimed || len(m.Channels) == 0 {
		return nil
	}
	a.provisionalTeamID = m.TeamID
	a.provisionalTeamName = m.TeamName
	a.bootstrap.DismissForCached()
	if m.Theme != "" {
		styles.Apply(m.Theme, a.themeOverrides)
		a.invalidateAllWinModelCaches()
		a.threadPanel.InvalidateCache()
		a.sidebar.InvalidateCache()
		a.compose.RefreshStyles()
		a.threadCompose.RefreshStyles()
	}
	if m.SidebarWidth != 0 {
		a.sidebar.SetWidth(m.SidebarWidth)
	}
	a.view = ViewChannels
	a.SetChannels(m.Channels)
	a.channelFinder.SetItems(m.FinderItems)
	a.activeTeamID = m.TeamID
	a.workspaceRail.SelectByID(m.TeamID)
	a.statusbar.SetBootSyncing(true)
	// A queued startup navigation (permalink, pane-state thread) owns
	// the first selection; it needs the connected workspace, so leave
	// it to the WorkspaceReadyMsg path.
	if a.startupLinkNav != nil {
		return nil
	}
	// Select only the restored channel. With no recorded restore the
	// real path opens the network list's first entry, whose identity
	// the cache (name-ordered) cannot predict.
	if m.LastChannelID == "" {
		return nil
	}
	for _, ch := range m.Channels {
		if ch.ID == m.LastChannelID {
			a.sidebar.SelectByID(ch.ID)
			id, name, chType := ch.ID, ch.Name, ch.Type
			return func() tea.Msg { return ChannelSelectedMsg{ID: id, Name: name, Type: chType} }
		}
	}
	return nil
}

// DismissForCached hides the connecting overlay so cache-painted
// content is visible; entries keep their "connecting" status for the
// later MarkReady / MarkFailed.
func (b *workspaceBootstrap) DismissForCached() {
	b.loading = false
}

func (a *App) clearProvisional() {
	a.provisionalTeamID = ""
	a.provisionalTeamName = ""
	a.statusbar.SetBootSyncing(false)
}

// clearProvisionalOnFailed drops the "cached · syncing" pill when the
// workspace it advertises fails to connect — nothing is in flight for
// it any more. Name-keyed because WorkspaceFailedMsg carries no team
// ID.
func (a *App) clearProvisionalOnFailed(teamName string) {
	if a.provisionalTeamID != "" && teamName == a.provisionalTeamName {
		a.clearProvisional()
	}
}

// reconcileProvisionalSelection is the WorkspaceReadyMsg channel-select
// arm for a workspace that already painted provisionally: keep the
// channel the user is on (they may have navigated during boot), skip
// the pane blanking, and re-run the selection through the tier
// dispatch — now with the workspace live, the bootstrap-persisted
// channel renders tier-1 fresh with no refetch, and any other channel
// gets the real verify fetch its provisional selection could not fire.
// ok=false means the activation should run the default path.
func (a *App) reconcileProvisionalSelection(m WorkspaceReadyMsg) (tea.Cmd, bool) {
	provisional := a.provisionalTeamID != "" && a.provisionalTeamID == m.TeamID
	a.clearProvisional()
	if !provisional || a.activeChannelID == "" || a.startupLinkNav != nil {
		return nil, false
	}
	name, chType, found := lookupChannelIn(a.activeChannelID, m.Channels, m.FinderItems)
	if !found {
		return nil, false
	}
	id := a.activeChannelID
	a.sidebar.SelectByID(id)
	return func() tea.Msg { return ChannelSelectedMsg{ID: id, Name: name, Type: chType} }, true
}

// applyCachedLastRead pushes the channel's persisted last-read
// watermark into the focused pane on selection, so cache-rendered
// tiers (which never see a MessagesLoadedMsg) still draw the unread
// line. A channel with no recorded state clears the watermark — the
// previous channel's line must not survive the switch.
func (a *App) applyCachedLastRead(channelID string) {
	if a.readStateReader == nil {
		return
	}
	a.messagepane.SetLastReadTS(a.readStateReader()[channelID].LastReadTS)
}
