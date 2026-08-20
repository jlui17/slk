// internal/ui/reducer_links.go
//
// Link-open routing (issue #62 + in-app permalink navigation).
//
// OpenLinkMsg is the single place every link open flows through:
//   - Slack archive permalinks whose subdomain matches the active
//     workspace AND whose channel resolves via ChannelService.Lookup
//     navigate in-app: dispatch ChannelSelectedMsg, then complete via
//     pendingLinkNav once the channel's messages are loaded (select
//     the target ts, or open the thread panel for thread_ts links).
//   - Everything else opens in the OS browser (a.browserOpener).
//
// Completion hooks live in reducer_channels.go (ChannelSelectedMsg
// and MessagesLoadedMsg arms call completePendingLinkNav).
package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages"
)

// pendingLinkNav is the not-yet-completed tail of an in-app permalink
// navigation. Set by routeLink, consumed by completePendingLinkNav.
type pendingLinkNav struct {
	channelID string
	messageTS string
	threadTS  string // non-empty: open the thread panel instead of selecting
}

var reduceLinks reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(OpenLinkMsg)
	if !ok {
		return nil, false
	}
	return a.routeLink(m.URL), true
}

// routeLink decides between in-app navigation and the browser.
func (a *App) routeLink(rawURL string) tea.Cmd {
	pl, ok := slackurl.Parse(rawURL)
	if !ok {
		debuglog.General("routeLink: not a permalink, browser: %s", rawURL)
		return a.browserOpener(rawURL)
	}
	domain := a.activeWorkspaceDomain()
	if domain == "" || pl.Subdomain != domain {
		debuglog.General("routeLink: domain %q != active %q, browser: %s", pl.Subdomain, domain, rawURL)
		return a.browserOpener(rawURL)
	}
	name, chType, found := a.channels.Lookup(pl.ChannelID)
	if !found {
		debuglog.General("routeLink: channel %s not found, browser: %s", pl.ChannelID, rawURL)
		return a.browserOpener(rawURL)
	}
	a.pendingLinkNav = &pendingLinkNav{
		channelID: string(pl.ChannelID),
		messageTS: string(pl.MessageTS),
		threadTS:  string(pl.ThreadTS),
	}
	debuglog.General("routeLink: in-app nav channel=%s ts=%s thread_ts=%s active=%s",
		pl.ChannelID, pl.MessageTS, pl.ThreadTS, a.activeChannelID)
	if string(pl.ChannelID) == a.activeChannelID {
		// Already viewing the channel; the loaded buffer is as good
		// as it gets, so complete authoritatively right now.
		return a.completePendingLinkNav(a.activeChannelID, true)
	}
	id, n, t := string(pl.ChannelID), name, chType
	return func() tea.Msg {
		return ChannelSelectedMsg{ID: id, Name: n, Type: t}
	}
}

// completePendingLinkNav finishes (or drops) the pending permalink
// navigation for channelID. authoritative=true means "no more message
// data is coming for this channel" — if the target ts still isn't in
// the buffer, dispatch ChannelService.FetchAround to load a history
// window centered on the target instead of waiting.
//
// Called from: routeLink (already-active channel, authoritative),
// reduceChannels' ChannelSelectedMsg arm (cache render, best-effort),
// and reduceChannels' MessagesLoadedMsg arm (authoritative).
func (a *App) completePendingLinkNav(channelID string, authoritative bool) tea.Cmd {
	p := a.pendingLinkNav
	if p == nil {
		return nil
	}
	if p.channelID != channelID {
		// The user navigated somewhere unrelated before the link
		// target finished loading; the pending nav is stale.
		debuglog.General("completePendingLinkNav: stale (pending=%s got=%s), dropped", p.channelID, channelID)
		a.pendingLinkNav = nil
		return nil
	}
	if p.threadTS != "" {
		debuglog.General("completePendingLinkNav: opening thread %s select=%s", p.threadTS, p.messageTS)
		a.pendingLinkNav = nil
		return a.openThreadForPermalink(p.channelID, p.threadTS, p.messageTS)
	}
	debuglog.General("completePendingLinkNav: channel=%s ts=%s authoritative=%v", channelID, p.messageTS, authoritative)
	// The select lands in the messages pane; with a thread panel open
	// (or zoomed) that pane is hidden or unfocused and the jump would
	// be invisible. Close the panel and focus the pane, mirroring the
	// cross-channel path (reduceChannelSelected closes the thread on
	// every switch). Ordering: CloseThread clears pane selections, so
	// it must precede SelectByTS.
	if a.threadVisible {
		a.CloseThread()
	}
	a.focusedPanel = PanelMessages
	if a.messagepane.SelectByTS(p.messageTS) {
		// A best-effort (cache-render) select is not the end of the
		// nav: the in-flight fetch's MessagesLoadedMsg will replace
		// the buffer via SetMessages, which clears the selection and
		// snaps to the bottom. Keep the nav armed so the authoritative
		// completion re-selects the target on the fresh buffer; only
		// an authoritative success retires it.
		if authoritative {
			a.pendingLinkNav = nil
		}
		return nil
	}
	if authoritative {
		a.pendingLinkNav = nil
		channels := a.channels
		chID, ts := p.channelID, p.messageTS
		return func() tea.Msg {
			return channels.FetchAround(ids.ChannelID(chID), ids.MessageTS(ts))
		}
	}
	return nil
}

// openThreadForPermalink opens the thread panel for a permalink that
// carried thread_ts. Unlike openThreadForSelectedMessage it does not
// require the parent message to be in the pane buffer (mirrors
// openSelectedThreadCmd, which builds the parent from a summary):
// the parent row is taken from the loaded buffer or the thread cache
// when available, else a minimal stub that the ThreadRepliesLoadedMsg
// handler backfills from cache once the fetch lands.
func (a *App) openThreadForPermalink(channelID, threadTS, selectTS string) tea.Cmd {
	parent := messages.MessageItem{TS: threadTS, ThreadTS: threadTS}
	if channelID == a.activeChannelID {
		for _, m := range a.messagepane.Messages() {
			if m.TS == threadTS {
				parent = m
				break
			}
		}
	}
	if parent.Text == "" {
		if cached := a.threads.CacheRead(ids.ChannelID(channelID), ids.ThreadTS(threadTS)); len(cached) > 0 {
			parent = cached[0]
		}
	}

	cmd := a.openThreadPanel(parent, channelID, threadTS)
	// Pin the cursor to the exact linked message across the panel's
	// reloads (cache prime, authoritative fetch) — otherwise each
	// SetThread snaps to the newest reply. Armed after openThreadPanel:
	// its SetThread clears the pin on a thread-identity change.
	a.threadPanel.SetPendingSelectTS(selectTS)
	return cmd
}
