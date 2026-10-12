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
// selects messageTS and opens its thread panel, replies or none (or
// opens the thread panel of threadTS when that is set, as the
// pane-state restore of an open thread always does) instead of
// restoring the last-visited channel. Call before the
// program starts; the caller is responsible for making the link's
// workspace the initial active one.
func (a *App) SetStartupLink(channelID, messageTS, threadTS string) {
	a.startupLinkNav = &pendingLinkNav{
		channelID:        channelID,
		messageTS:        messageTS,
		threadTS:         threadTS,
		openParentThread: true,
		openEmptyThread:  true,
	}
}

// HerdrTarget is where O and alt+o open a link: in a second slk
// instance in a new herdr tab or a new herdr pane. HerdrNone is o.
type HerdrTarget int

const (
	HerdrNone HerdrTarget = iota
	HerdrTab
	HerdrPane
)

func (t HerdrTarget) String() string {
	switch t {
	case HerdrTab:
		return "tab"
	case HerdrPane:
		return "pane"
	}
	return "none"
}

// SetHerdrTabOpener installs the callback that opens a permalink in a
// new herdr tab (the O keybinding). Installed only when slk runs in a
// herdr pane whose space is known; unset, O routes like o.
func (a *App) SetHerdrTabOpener(open func(url, label string, focus bool) error) {
	a.herdrTabOpener = open
}

// SetHerdrPaneOpener installs the callback that opens a permalink in a
// new herdr pane split to the right of splitPaneID, slk's own pane when
// empty, and returns the new pane's id (the alt+o keybinding).
// Installed whenever slk runs in a herdr pane; unset, alt+o routes like
// o.
func (a *App) SetHerdrPaneOpener(open func(url, splitPaneID string, focus bool) (string, error)) {
	a.herdrPaneOpener = open
}

// herdrOpener is an opener for target, nil when none is installed. Each
// call gives a fresh one, used for one open or one batch. A pane opener
// splits slk's pane first and then the pane its last successful open
// created, so a batch reads left to right in list order and slk keeps
// half the tab; the label goes unused, as the pane shares slk's tab.
func (a *App) herdrOpener(target HerdrTarget) func(url, label string, focus bool) error {
	switch {
	case target == HerdrTab && a.herdrTabOpener != nil:
		return a.herdrTabOpener
	case target == HerdrPane && a.herdrPaneOpener != nil:
		open, split := a.herdrPaneOpener, ""
		return func(url, _ string, focus bool) error {
			paneID, err := open(url, split, focus)
			if err == nil {
				split = paneID
			}
			return err
		}
	}
	return nil
}

// openInHerdr is routeLink's herdr arm: rawURL, a permalink slk can
// open, opens in a focused new tab labeled label, or a focused new
// pane. False when target has no opener installed.
func (a *App) openInHerdr(target HerdrTarget, rawURL, label string) (tea.Cmd, bool) {
	opener := a.herdrOpener(target)
	if opener == nil {
		return nil, false
	}
	debuglog.General("routeLink: herdr %v nav url=%s label=%s", target, rawURL, label)
	return func() tea.Msg {
		if err := opener(rawURL, label, true); err != nil {
			debuglog.Notify("herdr: open %v: %v", target, err)
			return ToastMsg{Text: "Failed to open herdr " + target.String()}
		}
		return nil
	}, true
}

// OpenLinksInHerdrMsg opens each permalink in its own herdr tab or pane
// (Target), in order, without moving herdr's focus off this slk.
// Dispatched by the O or alt+o link picker when rows are marked; every
// URL comes from that picker's list, which holds only links
// linkOpensInApp accepts.
type OpenLinksInHerdrMsg struct {
	URLs   []string
	Target HerdrTarget
}

var reduceOpenLinksInHerdr reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(OpenLinksInHerdrMsg)
	if !ok {
		return nil, false
	}
	return a.openLinksInHerdr(m.URLs, m.Target), true
}

// openLinksInHerdr opens the tabs or panes one after the other in a
// single cmd: the opener blocks until one's command is sent, so a loop
// is what makes them come out in list order. A failed open doesn't
// stop the rest; one toast reports the count.
func (a *App) openLinksInHerdr(urls []string, target HerdrTarget) tea.Cmd {
	type link struct{ url, label string }
	var links []link
	for _, u := range urls {
		if label, ok := a.herdrLinkLabel(u); ok {
			links = append(links, link{u, label})
		}
	}
	opener, total, noun := a.herdrOpener(target), len(urls), "herdr "+target.String()
	return func() tea.Msg {
		opened := 0
		for _, l := range links {
			if err := opener(l.url, l.label, false); err != nil {
				debuglog.Notify("herdr: open %v: %v", target, err)
				continue
			}
			opened++
		}
		switch {
		case opened < total:
			return ToastMsg{Text: fmt.Sprintf("Opened %d of %d %ss", opened, total, noun)}
		case total == 1:
			return ToastMsg{Text: "Opened 1 " + noun}
		default:
			return ToastMsg{Text: fmt.Sprintf("Opened %d %ss", total, noun)}
		}
	}
}

// herdrLinkLabel is the label routeLink hands rawURL's herdr opener: its
// channel's name. False for a link routeLink would hand to the browser.
func (a *App) herdrLinkLabel(rawURL string) (string, bool) {
	if !a.linkOpensInApp(rawURL) {
		return "", false
	}
	pl, _ := slackurl.Parse(rawURL)
	name, _, _ := a.channels.Lookup(pl.ChannelID)
	return name, true
}

// applyLinkPreview fills one picker row with its fetched message
// preview: the text, and "#channel · sender" in the Side column. Drops
// stale generations and results arriving after the picker closed or
// reopened for something other than link rows (`o`/`O` open them, `c`
// copies one).
func (a *App) applyLinkPreview(m LinkPreviewMsg) {
	linkRows := a.pickerKind == "links" || a.pickerKind == "copy"
	if m.Gen != a.linkPreviewGen || !linkRows || !a.linkPicker.IsVisible() {
		return
	}
	text := a.flattenRootText(m.Text)
	if text == "" {
		// Raw mrkdwn that flattens to nothing (whitespace, bare
		// entity tokens): the date-bearing fallback row beats an
		// empty one.
		return
	}
	a.linkPicker.SetDisplay(m.Index, text)
	name, chType, found := a.channels.Lookup(ids.ChannelID(m.ChannelID))
	if found && m.Sender != "" {
		a.linkPicker.SetSide(m.Index, channelDisplayName(name, chType)+" · "+m.Sender)
	}
}
