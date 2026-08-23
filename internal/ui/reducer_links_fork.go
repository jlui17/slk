package ui

import (
	"github.com/gammons/slk/internal/ids"
)

// SetStartupLink queues a permalink navigation for startup: once the
// initial active workspace is ready, the app opens channelID and
// selects messageTS (or opens the thread panel when threadTS is set)
// instead of restoring the last-visited channel. Call before the
// program starts; the caller is responsible for making the link's
// workspace the initial active one.
func (a *App) SetStartupLink(channelID, messageTS, threadTS string) {
	a.startupLinkNav = &pendingLinkNav{
		channelID:        channelID,
		messageTS:        messageTS,
		threadTS:         threadTS,
		openParentThread: true,
	}
}

// SetHerdrTabOpener installs the callback that opens a permalink in a
// new herdr tab (the O keybinding). Installed only when slk runs in a
// herdr pane whose space is known; unset, O routes like o.
func (a *App) SetHerdrTabOpener(open func(url, label string) error) {
	a.herdrTabOpener = open
}

// applyLinkPreview fills one picker row with its fetched message
// preview ("#channel · sender: text"). Drops stale generations and
// results arriving after the picker closed or reopened for files.
func (a *App) applyLinkPreview(m LinkPreviewMsg) {
	if m.Gen != a.linkPreviewGen || a.pickerKind != "links" || !a.linkPicker.IsVisible() {
		return
	}
	text := a.flattenRootText(m.Text)
	if text == "" {
		// Raw mrkdwn that flattens to nothing (whitespace, bare
		// entity tokens): the date-bearing fallback row beats a
		// dangling "sender: ".
		return
	}
	if sender := a.userNameFor(m.UserID); sender != "" {
		text = sender + ": " + text
	}
	if name, chType, found := a.channels.Lookup(ids.ChannelID(m.ChannelID)); found {
		text = channelDisplayName(name, chType) + " · " + text
	}
	a.linkPicker.SetDisplay(m.Index, text)
}
