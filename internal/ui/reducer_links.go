// internal/ui/reducer_links.go
//
// Link-open routing (issue #62 + in-app permalink navigation).
//
// OpenLinkMsg is the single place every link open flows through:
//   - Slack archive permalinks whose subdomain matches the active
//     workspace AND whose channel resolves via ChannelService.Lookup
//     navigate in-app: dispatch ChannelSelectedMsg, then complete via
//     pendingLinkNav once the channel's messages are loaded (open the
//     thread panel for thread_ts links and for targets that turn out
//     to be thread parents, else select the target ts in-channel).
//   - The same permalinks with InHerdrTab set (the O keybinding) go to
//     a.herdrTabOpener instead when one is installed: a new herdr tab
//     running a second slk instance on the link.
//   - Everything else opens in the OS browser (a.browserOpener).
//
// LinkPreviewMsg (the picker's async permalink previews) also lands
// here: applyLinkPreview fills the row it targets.
//
// Completion hooks live in reducer_channels.go (the ChannelSelectedMsg
// and MessagesLoadedMsg arms call completePendingLinkNav; the
// MessagesAroundLoadedMsg arm retires a nav that rode through
// FetchAround).
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
	// openParentThread: a target that turns out to be a thread parent
	// opens its thread panel. True for permalink navs (a parent's
	// permalink carries no thread_ts, but following it means the
	// thread); false for workspace-search jumps, where a top-level hit
	// should stay a plain in-channel select.
	openParentThread bool
	// delivered flips once the jump has visibly landed (a SelectByTS
	// succeeded). A nav kept armed past that point only re-selects on
	// the fresh buffer; it must not re-yank focus or close a thread
	// panel the user opened in the meantime.
	delivered bool
}

var reduceLinks reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case OpenLinkMsg:
		return a.routeLink(m.URL, m.InHerdrTab), true
	case LinkPreviewMsg:
		a.applyLinkPreview(m)
		return nil, true
	}
	return nil, false
}

// routeLink decides between in-app navigation, a new herdr tab, and
// the browser.
func (a *App) routeLink(rawURL string, inHerdrTab bool) tea.Cmd {
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
	if inHerdrTab && a.herdrTabOpener != nil {
		opener, label := a.herdrTabOpener, name
		debuglog.General("routeLink: herdr tab nav channel=%s label=%s", pl.ChannelID, name)
		return func() tea.Msg {
			if err := opener(rawURL, label); err != nil {
				debuglog.Notify("herdr: open tab: %v", err)
				return ToastMsg{Text: "Failed to open herdr tab"}
			}
			return nil
		}
	}
	a.pendingLinkNav = &pendingLinkNav{
		channelID:        string(pl.ChannelID),
		messageTS:        string(pl.MessageTS),
		threadTS:         string(pl.ThreadTS),
		openParentThread: true,
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
// window centered on the target instead of waiting; the nav stays
// armed and the MessagesAroundLoadedMsg arm finishes it.
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
	// every switch). Only until the jump first lands: a re-completion
	// (the authoritative pass after a best-effort success) just
	// re-selects, without tearing down whatever the user opened since.
	// Ordering: CloseThread clears pane selections, so it must precede
	// SelectByTS.
	firstLanding := !p.delivered
	if firstLanding {
		if a.threadVisible {
			a.CloseThread()
		}
		a.focusedPanel = PanelMessages
	}
	if a.messagepane.SelectByTS(p.messageTS) {
		p.delivered = true
		// A thread parent's permalink carries no thread_ts, but
		// following it means the thread: when the target has replies,
		// open its thread panel (cursor on the parent row) instead of
		// stopping at the in-channel select. Permalink navs only
		// (openParentThread): a workspace-search jump to a top-level
		// hit stays a plain select. Re-evaluated on every pass while no
		// thread panel is up: the first landing is usually the cached
		// render, whose parent row can carry a stale zero ReplyCount
		// (live replies bump only the in-memory pane, never the cache
		// row), so only the authoritative buffer's count is
		// trustworthy. An already-open panel — this nav's own
		// best-effort open, or one the user raised mid-flight — stays
		// put.
		if p.openParentThread && !a.threadVisible {
			for _, m := range a.messagepane.Messages() {
				if m.TS != p.messageTS {
					continue
				}
				if m.ReplyCount > 0 {
					debuglog.General("completePendingLinkNav: target is a thread parent (%d replies), opening thread", m.ReplyCount)
					if authoritative {
						a.pendingLinkNav = nil
					}
					return a.openThreadForPermalink(p.channelID, p.messageTS, p.messageTS)
				}
				break
			}
		}
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
		// The nav stays armed: the target is off-buffer, so whether it
		// is a thread parent is unknowable until the fetched window
		// lands. The MessagesAroundLoadedMsg arm finishes (or drops)
		// the nav.
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
	// Pin the cursor to the exact linked message: the panel loads in
	// passes (cache prime, authoritative fetch) and the pin holds until
	// the pass that contains the message, then disarms so later
	// refetches can't yank the cursor back. Armed after openThreadPanel:
	// its SetThread drops the pin on a thread-identity change.
	a.threadPanel.SetPendingSelectTS(selectTS)
	return cmd
}
