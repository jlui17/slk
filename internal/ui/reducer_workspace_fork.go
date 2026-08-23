package ui

import (
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// lookupChannelIn resolves channelID against the workspace data carried
// on WorkspaceReadyMsg, in ChannelService.Lookup's scan order (sidebar
// items, then finder items): the msg is the exact snapshot this arm is
// applying, so the result can't diverge from what gets rendered.
func lookupChannelIn(channelID string, channels []sidebar.ChannelItem, finder []channelfinder.Item) (name, chType string, ok bool) {
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
