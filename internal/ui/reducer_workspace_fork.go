package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// promoteStartupLinkNav lets a queued startup navigation (a
// command-line permalink, or a pane-state thread being restored)
// override reduceWorkspaceReady's channel restore: it becomes
// pendingLinkNav, so the completion hooks in reducer_channels.go select
// the target message / open the thread panel, exactly as an in-app link
// click would, and *target becomes its channel. A channel m doesn't
// hold leaves *target as it was and returns a toast.
func (a *App) promoteStartupLinkNav(target *sidebar.ChannelItem, m WorkspaceReadyMsg) tea.Cmd {
	nav := a.startupLinkNav
	if nav == nil {
		return nil
	}
	a.startupLinkNav = nil
	name, chType, found := lookupChannelIn(string(nav.ChannelID), m.Channels, m.FinderItems)
	if !found {
		return func() tea.Msg {
			return ToastMsg{Text: "Linked channel not found in this workspace"}
		}
	}
	a.pendingLinkNav = nav
	*target = sidebar.ChannelItem{ID: string(nav.ChannelID), Name: name, Type: chType}
	return nil
}

// lookupChannelIn resolves channelID against the workspace data carried
// on WorkspaceReadyMsg, in ChannelService.Lookup's scan order (sidebar
// items, then finder items): the msg is the exact snapshot this arm is
// applying, so the result can't diverge from what gets rendered.
func lookupChannelIn(channelID string, channels []sidebar.ChannelItem, finder []core.ChannelFinderItem) (name, chType string, ok bool) {
	for _, ch := range channels {
		if ch.ID == channelID {
			return ch.Name, ch.Type, true
		}
	}
	for _, it := range finder {
		if it.ID == channelID {
			return it.Name, it.Type, true
		}
	}
	return "", "", false
}
