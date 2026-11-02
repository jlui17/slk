package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
)

// channelLink is the conversation a link to it names
// (https://<sub>.slack.com/archives/<C>), when it is one of this
// workspace's that slk knows.
func (a *App) channelLink(rawURL string) (ids.ChannelID, bool) {
	sub, channelID, ok := slackurl.ParseChannel(rawURL)
	if !ok || sub != a.activeWorkspaceDomain() {
		return "", false
	}
	_, _, found := a.channels.Lookup(channelID)
	return channelID, found
}

// routeChannelLink is routeLink's arm for a link to a conversation: it
// opens the conversation in slk, as Slack opens it. O and alt+o open it
// here too, as slk in a new herdr tab or pane starts only on a
// message's link.
func (a *App) routeChannelLink(rawURL string) (tea.Cmd, bool) {
	channelID, ok := a.channelLink(rawURL)
	if !ok {
		return nil, false
	}
	return a.applyLocation(Location{TeamID: ids.TeamID(a.activeTeamID), ChannelID: channelID}, false)
}

// opensInSlk reports whether routeLink opens rawURL in slk rather than
// the browser: a message's permalink or a conversation of this
// workspace.
func (a *App) opensInSlk(rawURL string) bool {
	_, ok := a.channelLink(rawURL)
	return ok || a.linkOpensInApp(rawURL)
}
