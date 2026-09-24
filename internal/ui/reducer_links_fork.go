package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
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
func (a *App) SetHerdrTabOpener(open func(url, label string, focus bool) error) {
	a.herdrTabOpener = open
}

// OpenLinksInHerdrTabsMsg opens each permalink in its own herdr tab, in
// order, without moving herdr's focus off this slk. Dispatched by the
// O link picker when rows are marked; every URL comes from that
// picker's list, which holds only links linkOpensInApp accepts.
type OpenLinksInHerdrTabsMsg struct{ URLs []string }

var reduceOpenLinksInHerdrTabs reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(OpenLinksInHerdrTabsMsg)
	if !ok {
		return nil, false
	}
	return a.openLinksInHerdrTabs(m.URLs), true
}

// openLinksInHerdrTabs opens the tabs one after the other in a single
// cmd: the opener blocks until a tab's command is sent, so a loop is
// what makes the tabs come out in list order. A failed open doesn't
// stop the rest; one toast reports the count.
func (a *App) openLinksInHerdrTabs(urls []string) tea.Cmd {
	type tab struct{ url, label string }
	var tabs []tab
	for _, u := range urls {
		if label, ok := a.herdrTabLabel(u); ok {
			tabs = append(tabs, tab{u, label})
		}
	}
	opener, total := a.herdrTabOpener, len(urls)
	return func() tea.Msg {
		opened := 0
		for _, t := range tabs {
			if err := opener(t.url, t.label, false); err != nil {
				debuglog.Notify("herdr: open tab: %v", err)
				continue
			}
			opened++
		}
		switch {
		case opened < total:
			return ToastMsg{Text: fmt.Sprintf("Opened %d of %d herdr tabs", opened, total)}
		case total == 1:
			return ToastMsg{Text: "Opened 1 herdr tab"}
		default:
			return ToastMsg{Text: fmt.Sprintf("Opened %d herdr tabs", total)}
		}
	}
}

// herdrTabLabel is the label routeLink gives rawURL's herdr tab: its
// channel's name. False for a link routeLink would hand to the browser.
func (a *App) herdrTabLabel(rawURL string) (string, bool) {
	if !a.linkOpensInApp(rawURL) {
		return "", false
	}
	pl, _ := slackurl.Parse(rawURL)
	name, _, _ := a.channels.Lookup(pl.ChannelID)
	return name, true
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
