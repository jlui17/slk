// internal/ui/reducer_links.go
//
// Link-open routing (issue #62 + in-app permalink navigation).
//
// OpenLinkMsg is the single place every link open flows through:
//   - Slack archive permalinks whose subdomain matches the active
//     workspace AND whose channel resolves via ChannelService.Lookup
//     navigate in-app through applyLocation: dispatch
//     ChannelSelectedMsg, then complete via pendingLinkNav once the
//     channel's messages are loaded (open the thread panel for
//     thread_ts links and for targets that turn out to be thread
//     parents, else select the target ts in-channel).
//   - The same permalinks with InHerdr set (the O and alt+o keybindings)
//     open a second slk instance on the link instead, in a new herdr tab
//     or pane, when that target's opener is installed.
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
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages"
)

// pendingLinkNav is the not-yet-completed tail of an in-app navigation
// (a permalink, a mark, a history walk, a search jump). The embedded
// Location is the target (ThreadTS non-empty: open the thread panel and
// select within it); the rest says how it lands. Set by applyLinkNav,
// consumed by completePendingLinkNav.
type pendingLinkNav struct {
	Location
	// openParentThread: a target that turns out to be a thread parent
	// opens its thread panel. True for permalink navs (a parent's
	// permalink carries no thread_ts, but following it means the
	// thread); false for workspace-search jumps, marks and history
	// walks, where a top-level target should stay a plain in-channel
	// select.
	openParentThread bool
	// delivered flips once the jump has visibly landed (a SelectByTS
	// succeeded). A nav kept armed past that point only re-selects on
	// the fresh buffer; it must not re-yank focus or close a thread
	// panel the user opened in the meantime.
	delivered bool
	// Fork: openEmptyThread opens the thread panel of a target nobody
	// has replied to as well, parent row only, ready for the first
	// reply. True for O, alt+o and a startup link (`slk <link>`, what O
	// and alt+o run in their herdr tab or pane); needs openParentThread.
	openEmptyThread bool
}

var reduceLinks reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case OpenLinkMsg:
		return a.routeLink(m.URL, m.InHerdr), true
	case LinkPreviewMsg:
		a.applyLinkPreview(m)
		return nil, true
	}
	return nil, false
}

// routeLink decides between in-app navigation, a new herdr tab or pane,
// and the browser.
func (a *App) routeLink(rawURL string, inHerdr HerdrTarget) tea.Cmd {
	if cmd, ok := a.routeProfileLink(rawURL, inHerdr); ok { // fork: profilecard.go
		return cmd
	}
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
	name, _, found := a.channels.Lookup(pl.ChannelID)
	if !found {
		debuglog.General("routeLink: channel %s not found, browser: %s", pl.ChannelID, rawURL)
		return a.browserOpener(rawURL)
	}
	if cmd, ok := a.openInHerdr(inHerdr, rawURL, name); ok { // fork: reducer_links_fork.go
		return cmd
	}
	debuglog.General("routeLink: in-app nav channel=%s ts=%s thread_ts=%s active=%s",
		pl.ChannelID, pl.MessageTS, pl.ThreadTS, a.activeChannelID)
	cmd, ok := a.applyLinkNav(&pendingLinkNav{
		Location: Location{
			TeamID:    ids.TeamID(a.activeTeamID),
			ChannelID: pl.ChannelID,
			MessageTS: pl.MessageTS,
			ThreadTS:  pl.ThreadTS,
		},
		openParentThread: true,
		openEmptyThread:  inHerdr != HerdrNone,
	}, false)
	if ok {
		return cmd
	}
	return a.browserOpener(rawURL)
}

// applyLocation is the single applier every in-app jump flows through
// — a permalink, a mark, or a history walk. It records loc as the
// pending navigation and dispatches the channel switch, so
// completePendingLinkNav can finish once the target channel's messages
// land (or immediately when the channel is already active). ok=false
// means loc's channel could not be resolved and nothing was started;
// while an upload is in flight nothing is started either, and the
// returned cmd is the "Upload in progress" toast with ok=true;
// callers that need a fallback (routeLink's browser opener, a mark-jump
// toast) branch on ok. A nil cmd with ok=true is a completed navigation
// (e.g. SelectByTS selected the message in the already-active channel),
// not a failure.
//
// fromHistory marks the synthesized ChannelSelectedMsg so the reducer
// does not record the walk as a new visit — history walks must not
// grow the stack. It also forces the ChannelSelectedMsg path even when
// the target channel is already active: a walk is a navigation between
// recorded positions, not an in-place jump, so it always goes through
// the channel-switch pipeline (a same-channel walk is reachable in
// production when stale entries between two same-channel entries are
// skipped and dropped). Direct jumps (permalinks, marks) keep the
// in-place completion so re-selecting the current channel does not
// reload it.
func (a *App) applyLocation(loc Location, fromHistory bool) (tea.Cmd, bool) {
	return a.applyLinkNav(&pendingLinkNav{Location: loc}, fromHistory)
}

// applyLinkNav is applyLocation for a nav that carries the fork's
// landing flags (routeLink's permalink navs).
func (a *App) applyLinkNav(nav *pendingLinkNav, fromHistory bool) (tea.Cmd, bool) {
	loc := nav.Location
	if a.compose.Uploading() || a.threadCompose.Uploading() {
		return toastWithClear(a, "Upload in progress", 2*time.Second), true
	}
	name, chType, found := a.channels.Lookup(loc.ChannelID)
	if !found {
		return nil, false
	}
	a.pendingLinkNav = nav
	if !fromHistory && string(loc.ChannelID) == a.activeChannelID {
		// Already viewing the channel; unless its fetch is still
		// running (below), the loaded buffer is as good as it gets,
		// so complete authoritatively right now. The view and focus
		// reset ChannelSelectedMsg would have done happens in
		// completePendingLinkNav's first landing.
		authoritative := a.channelFetchInFlight != a.activeChannelID
		return a.completePendingLinkNav(a.activeChannelID, authoritative), true
	}
	id, n, t, team := string(loc.ChannelID), name, chType, a.activeTeamID
	return func() tea.Msg {
		return ChannelSelectedMsg{ID: id, Name: n, Type: t, FromHistory: fromHistory, TeamID: team}
	}, true
}

// completePendingLinkNav finishes (or drops) the pending navigation for
// channelID. authoritative=true means "no more message data is coming
// for this channel" — if the target ts still isn't in the buffer,
// dispatch ChannelService.FetchAround to load a history window centered
// on the target instead of waiting; the nav stays armed and the
// MessagesAroundLoadedMsg arm finishes it.
//
// Called from: applyLinkNav (already-active channel), reduceChannels'
// ChannelSelectedMsg arm (cache render, best-effort), and
// reduceChannels' MessagesLoadedMsg arm (authoritative).
func (a *App) completePendingLinkNav(channelID string, authoritative bool) tea.Cmd {
	p := a.pendingLinkNav
	if p == nil {
		return nil
	}
	if string(p.ChannelID) != channelID || (p.TeamID != "" && string(p.TeamID) != a.activeTeamID) {
		// The user navigated somewhere unrelated before the link
		// target finished loading; the pending nav is stale.
		debuglog.General("completePendingLinkNav: stale (pending=%s got=%s), dropped", p.ChannelID, channelID)
		a.pendingLinkNav = nil
		return nil
	}
	if p.MessageTS == "" && p.ThreadTS == "" {
		// Channel-only location: the channel is already (being)
		// opened and there is nothing to select or open on top of it.
		// Permalinks and search hits always carry a message ts, so
		// only history walks produce this — and FetchAround with an
		// empty ts would be a bogus request.
		a.pendingLinkNav = nil
		return nil
	}
	firstLanding := !p.delivered
	if firstLanding {
		// The target is in the channel, also when the jump was taken
		// from the Threads or Activity view (an app's Home keeps its
		// view: apphome.go routes its own DM).
		if a.view != ViewAppHome {
			a.view = ViewChannels
			a.sidebar.SetThreadsActive(false)
		}
		a.lastOpenedChannelID = ""
		a.lastOpenedThreadTS = ""
	}
	if p.ThreadTS != "" {
		debuglog.General("completePendingLinkNav: opening thread %s select=%s", p.ThreadTS, p.MessageTS)
		a.pendingLinkNav = nil
		return a.openThreadForPermalink(string(p.ChannelID), string(p.ThreadTS), string(p.MessageTS))
	}
	debuglog.General("completePendingLinkNav: channel=%s ts=%s authoritative=%v", channelID, p.MessageTS, authoritative)
	// The select lands in the messages pane; with a thread panel open
	// (or zoomed) that pane is hidden or unfocused and the jump would
	// be invisible. Close the panel and focus the pane, mirroring the
	// cross-channel path (reduceChannelSelected closes the thread on
	// every switch). Only until the jump first lands: a re-completion
	// (the authoritative pass after a best-effort success) just
	// re-selects, without tearing down whatever the user opened since.
	// Ordering: CloseThread clears pane selections, so it must precede
	// SelectByTS.
	if firstLanding {
		if a.threadVisible {
			a.CloseThread()
		}
		a.focusedPanel = PanelMessages
	}
	if a.messagepane.SelectByTS(string(p.MessageTS)) {
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
				if m.TS != string(p.MessageTS) {
					continue
				}
				// Fork: under openEmptyThread the count decides nothing,
				// so the thread opens on the first pass that holds the
				// target, a stale cached count included.
				if m.ReplyCount > 0 || p.openEmptyThread {
					debuglog.General("completePendingLinkNav: target is a thread parent (%d replies), opening thread", m.ReplyCount)
					if authoritative {
						a.pendingLinkNav = nil
					}
					// Fork: a reply that was also sent to the channel has
					// no replies either; its thread is its parent's.
					threadTS := string(p.MessageTS)
					if m.ThreadTS != "" {
						threadTS = m.ThreadTS
					}
					return a.openThreadForPermalink(string(p.ChannelID), threadTS, string(p.MessageTS))
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
		chID, ts := p.ChannelID, p.MessageTS
		return func() tea.Msg {
			return channels.FetchAround(chID, ts)
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
