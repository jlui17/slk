package ui

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/slack/mrkdwn"
	"github.com/gammons/slk/internal/ui/messages"
)

// AgentReportFunc mirrors the tracked agent thread (a thread whose root
// message mentions, or was written by, a bot user) onto an external agent
// sidebar. agent is the sidebar's
// internal agent id, displayName the human name it shows, title the pane
// title, working the in-progress flag, statusMessage the assistant's
// transient status text ("is thinking…", may be empty). See internal/herdr.
type AgentReportFunc func(agent, displayName, title string, working bool, statusMessage string)

// AgentUnreadReportFunc publishes the tracked thread's unread state as a
// synthetic working→idle completion (the only edge herdr derives its
// unseen "done" indicator from) with statusMessage as the row's status
// text. See internal/herdr.Reporter.ReportUnread.
type AgentUnreadReportFunc func(agent, displayName, title, statusMessage string)

// HerdrTabViewMsg is dispatched by the herdr focus watcher with this
// pane's viewed state: whether its tab is the focused herdr workspace's
// active tab. Sent once when the watcher connects and on every change
// after. Absent entirely outside herdr, which App.PaneViewed treats as
// viewed — there is no signal to say otherwise, and the conservative
// answer is the one that doesn't withhold what the user can see.
type HerdrTabViewMsg struct{ Viewed bool }

// HerdrConnectedMsg is dispatched each time the herdr focus watcher
// establishes its event subscription, the initial connection included. A
// reconnect means herdr was restarting or unreachable, so reports sent in
// the gap are gone (they are fire-and-forget); the tracked agent thread
// answers by republishing its current state.
type HerdrConnectedMsg struct{}

// PaneViewed reports whether the user can currently see what this pane
// renders: inside herdr, whether its tab is the focused workspace's
// active tab; outside herdr, always true, since nothing can say
// otherwise. Read state is the caller that matters — marking a thread
// read because its panel is open is only honest if the pane is on
// screen, and a pane parked in a background herdr tab is not.
func (a *App) PaneViewed() bool {
	return !a.herdrTabViewKnown || a.herdrTabViewed
}

// AgentTabNameFunc names the pane's surrounding tab after the open agent
// thread. The implementation decides whether the rename is allowed (it must
// never overwrite a label the user set themselves).
type AgentTabNameFunc func(label string)

// UserInfoFunc resolves a user ID against the user cache. ok is false when
// the user isn't cached yet; agent-thread detection then skips the candidate
// rather than blocking on a network round-trip.
type UserInfoFunc func(userID string) (displayName string, isBot, ok bool)

// AssistantStatusMsg mirrors an ai_assistant_status WS event: an AI
// assistant (Claude Tag) started or finished composing in a thread.
// Fires for every thread in the workspace, not just visible ones;
// a non-empty Status ("is thinking…") means the assistant's turn is
// in progress, an empty Status clears it.
type AssistantStatusMsg struct {
	// TeamID is the workspace the event arrived in. These fire for every
	// workspace, and a thread's channel and ts are unique only within
	// one, so the tag is what keeps a same-id thread elsewhere from
	// driving the tracked row's turn state.
	TeamID    string
	ChannelID string
	ThreadTS  string
	BotUserID string
	Status    string
}

// agentSidebar bundles the herdr agent-sidebar integration: the callbacks
// mirroring the tracked agent thread onto the sidebar, the tab renamer,
// the user lookup backing bot-user detection, and the tracking state
// itself. The callbacks are nil unless slk runs inside a herdr pane
// (see SetAgentReporter).
type agentSidebar struct {
	report       AgentReportFunc
	reportUnread AgentUnreadReportFunc
	nameTab      AgentTabNameFunc
	userInfo     UserInfoFunc
	thread       agentThreadState

	// labelGen and llmLabel drive the model-generated tab-label
	// refinement; see agentthread_llm.go.
	labelGen AgentTabLabelFunc
	llmLabel llmLabelState

	// working mirrors the assistant's turn state from the last
	// AssistantStatusMsg for the tracked thread; while true, unread
	// changes ride the next turn-end report instead of publishing a
	// completion that would stomp the live working state.
	working bool
	// statusText is the working state's transient message ("is
	// thinking…"), kept so a display refresh can republish the row
	// without blanking it.
	statusText string
	// unread holds the timestamps of the tracked thread's unread
	// replies per Slack's read state: added on arrival, dropped when
	// the reply is deleted, emptied when any device marks the thread
	// read. Timestamps rather than a bare count so a deletion can
	// remove exactly what it retracts; empty means read.
	unread map[string]struct{}
}

// agentThreadState identifies the tracked agent thread — the last thread
// opened in the thread panel whose root message mentions, or was written
// by, a bot user. Tracking outlives the panel and the workspace:
// navigating away, closing the panel, or switching workspaces all keep
// the entry, so the thread's unread state still has a sidebar row to land
// on; only a different agent thread replaces it. Zero value means no
// agent thread has been tracked.
type agentThreadState struct {
	active    bool
	channelID string
	threadTS  string
	botUserID string
	// selfID and teamID belong to the thread's own workspace, captured
	// at tracking time: tracking outlives a workspace switch, after
	// which App.currentUserID and App.activeTeamID name a different
	// workspace. teamID is what decides whether an incoming reply,
	// read-mark, assistant status, or connection change concerns this
	// thread at all; selfID identifies its own author's replies.
	selfID    string
	teamID    string
	agentName string
	title     string
}

// threadEventIsOurs reports whether an event tagged teamID concerns the
// tracked thread's workspace. An untagged event means the active
// workspace, per the message contracts, so it resolves rather than
// matching everything: with every workspace's traffic now reaching the
// reducers, a wildcard would let a same-id thread elsewhere drive the row.
func (a *App) threadEventIsOurs(teamID string) bool {
	if teamID == "" {
		teamID = a.activeTeamID
	}
	return teamID == a.agentSidebar.thread.teamID
}

// sameThread reports whether both states track the same thread, comparing
// identity only.
func (s agentThreadState) sameThread(other agentThreadState) bool {
	return other.active && s.channelID == other.channelID &&
		s.threadTS == other.threadTS && s.botUserID == other.botUserID
}

// SetAgentReporter installs the agent-sidebar callbacks and the user lookup
// backing bot-user detection. Installed only when slk runs inside a herdr
// pane; unset, agent-thread detection is inert.
func (a *App) SetAgentReporter(report AgentReportFunc, reportUnread AgentUnreadReportFunc, nameTab AgentTabNameFunc, userInfo UserInfoFunc) {
	a.agentSidebar.report = report
	a.agentSidebar.reportUnread = reportUnread
	a.agentSidebar.nameTab = nameTab
	a.agentSidebar.userInfo = userInfo
}

// setThreadPanel is the single path that changes the thread panel's content;
// agent-thread detection and pane-state recording ride on it so no present or
// future open path can skip them.
func (a *App) setThreadPanel(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	a.threadPanel.SetThread(parent, replies, channelID, threadTS)
	a.updateAgentThread(parent, channelID, threadTS)
	a.reportPaneState(channelID, threadTS)
}

// updateAgentThread re-evaluates agent-thread detection against the thread
// panel's root message and publishes the sidebar entry. Re-entry for the
// thread already tracked (a replies reload through setThreadPanel) only
// refreshes the display fields, so it can't stomp a live working state or
// a pending unread count; only a different thread resets them. A nil
// agentReport (slk not in a herdr pane, or integration disabled) makes it
// a no-op.
func (a *App) updateAgentThread(parent messages.MessageItem, channelID, threadTS string) {
	if a.agentSidebar.report == nil || a.agentSidebar.userInfo == nil {
		return
	}
	botUserID, name, ok := a.firstBotMention(parent.Text)
	if !ok {
		botUserID, name, ok = a.botUser(parent.UserID)
	}
	if !ok {
		// A non-agent thread doesn't end tracking: the entry stays on
		// the last agent thread so its unread state keeps a row to
		// land on (only another agent thread replaces it).
		return
	}
	flat := a.flattenRootText(parent.Text)
	next := agentThreadState{
		active:    true,
		channelID: channelID,
		threadTS:  threadTS,
		botUserID: botUserID,
		selfID:    a.currentUserID,
		teamID:    a.activeTeamID,
		agentName: name,
		title:     a.agentThreadTitle(channelID, flat),
	}
	// Identity, not the whole struct: agentName and title are derived from
	// caches that fill in asynchronously (channel names, mention
	// resolution, a permalink thread's backfilled root text), so comparing
	// them would read a late-resolving name as a thread switch and reset
	// live state mid-turn.
	if cur := a.agentSidebar.thread; next.sameThread(cur) {
		if next.agentName == cur.agentName && next.title == cur.title {
			return
		}
		// Keep the rest of cur (notably selfID and teamID, which belong
		// to the thread's own workspace) and refresh only what's shown.
		cur.agentName, cur.title = next.agentName, next.title
		a.agentSidebar.thread = cur
		a.reportAgentThreadState()
		a.maybeRequestAgentTabLabel(a.flattenRootText(stripMention(parent.Text, botUserID)))
		return
	}
	// herdr keys one sidebar entry per pane, so reporting a different agent
	// id replaces the previous entry — switching between two agents'
	// threads needs no cross-agent release.
	a.agentSidebar.thread = next
	a.agentSidebar.working = false
	// Opening the thread is what starts tracking, and the open path marks
	// it read, so tracking starts read.
	a.agentSidebar.unread = nil
	a.agentSidebar.llmLabel = llmLabelState{}
	// The initial state is idle: ai_assistant_status is edge-triggered, so
	// a turn already in progress isn't visible until its next event.
	a.agentSidebar.report(agentSidebarID(name), name, next.title, false, "")
	// The mention is dropped from the raw text, not trimmed from the
	// flattened string: trimming by rendered name breaks when the
	// in-memory name map and the user cache disagree on the bot's name.
	stripped := a.flattenRootText(stripMention(parent.Text, botUserID))
	if a.agentSidebar.nameTab != nil {
		a.agentSidebar.nameTab(agentTabLabel(stripped))
	}
	a.maybeRequestAgentTabLabel(stripped)
}

// stripMention removes every <@userID> mention (bare or labeled) from raw
// mrkdwn text.
func stripMention(text, userID string) string {
	re := regexp.MustCompile(`<@` + regexp.QuoteMeta(userID) + `(\|[^>]*)?>`)
	return re.ReplaceAllLiteralString(text, "")
}

// noteAgentThreadReply bumps the tracked thread's unread count when msg is
// a reply to it from someone else, and publishes the new state. It runs
// ahead of the new-message reducer's filtering, which it must to see
// background workspaces at all, so it owns every guard that decides
// whether a message is a new reply from someone else: an edit echo is
// not a new reply, and a reply the user wrote never leaves their own
// thread unread. Authorship is judged two ways because neither covers
// the other -- the thread's own workspace's user id, and the send dedup,
// which recognizes an slk-originated echo whatever id it carries.
func (a *App) noteAgentThreadReply(teamID, channelID string, msg messages.MessageItem) {
	t := a.agentSidebar.thread
	if !t.active || !a.threadEventIsOurs(teamID) || channelID != t.channelID || msg.ThreadTS != t.threadTS {
		return
	}
	if msg.TS == msg.ThreadTS || msg.IsEdited {
		return
	}
	if msg.UserID == t.selfID || a.selfSend.IsSelfSent(msg.TS) {
		return
	}
	a.agentSidebar.addUnread(msg.TS)
	a.reportAgentThreadUnread()
}

// markAgentThreadRead and markAgentThreadUnread apply a read-state change
// for (channelID, threadTS) to the tracked thread. Every path that decides
// a thread's read state — the local mark-read on open, the local
// mark-unread press, and remote thread_marked echoes from other devices —
// funnels through one of them, so the sidebar row mirrors Slack's read
// state wherever it changed.
func (a *App) markAgentThreadRead(teamID, channelID, threadTS string) {
	t := a.agentSidebar.thread
	if !a.tracksThread(teamID, channelID, threadTS) || a.agentSidebar.unreadTotal() == 0 {
		return
	}
	a.agentSidebar.unread = nil
	// Clearing goes through a plain idle report: idle→idle leaves
	// herdr's seen flag untouched, so a dot herdr is showing can
	// only be cleared by focusing the tab — but the row's status
	// text stops claiming unread immediately.
	if !a.agentSidebar.working && a.agentSidebar.report != nil {
		a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, false, "")
	}
}

// markAgentThreadUnread takes the mark's boundary timestamp: it carries a
// boundary rather than a count, so the boundary message is the one thing
// known to be unread until arrivals refine it.
func (a *App) markAgentThreadUnread(teamID, channelID, threadTS, boundaryTS string) {
	if !a.tracksThread(teamID, channelID, threadTS) || a.agentSidebar.unreadTotal() > 0 {
		return
	}
	a.agentSidebar.addUnread(boundaryTS)
	a.reportAgentThreadUnread()
}

// dropAgentThreadReply stops counting a reply that was deleted, in
// whichever workspace it was deleted.
func (a *App) dropAgentThreadReply(teamID, channelID, ts string) {
	t := a.agentSidebar.thread
	if !t.active || !a.threadEventIsOurs(teamID) || channelID != t.channelID ||
		!a.agentSidebar.dropUnread(ts) {
		return
	}
	if a.agentSidebar.unreadTotal() > 0 {
		a.reportAgentThreadUnread()
		return
	}
	if !a.agentSidebar.working && a.agentSidebar.report != nil {
		a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, false, "")
	}
}

// tracksThread reports whether an event for (teamID, channelID, threadTS)
// belongs to the thread currently tracked.
func (a *App) tracksThread(teamID, channelID, threadTS string) bool {
	t := a.agentSidebar.thread
	return t.active && a.threadEventIsOurs(teamID) &&
		channelID == t.channelID && threadTS == t.threadTS
}

// dropAgentThreadTurnState forgets the assistant's in-progress turn.
// ai_assistant_status is edge-triggered, so a connection that dropped
// mid-turn takes the turn-end event with it; without this the working
// latch would stay set forever and silently swallow every later unread
// publication. Deliberately publishes only when unread replies are
// waiting: with nothing to say, reporting idle would manufacture a
// working→idle edge and light the unseen indicator for a turn nobody
// knows the end of.
func (a *App) dropAgentThreadTurnState(teamID string) {
	t := a.agentSidebar.thread
	if !t.active || !a.agentSidebar.working {
		return
	}
	// Another workspace's connection blip says nothing about this
	// thread's assistant.
	if !a.threadEventIsOurs(teamID) {
		return
	}
	a.agentSidebar.working = false
	a.agentSidebar.statusText = ""
	if a.agentSidebar.unreadTotal() > 0 {
		a.reportAgentThreadUnread()
	}
}

// reportAgentThreadState republishes the tracked thread's current state
// without changing it, for when only the display fields moved (a channel
// name or mention that resolved late). Deliberately never a completion:
// a title refresh must not light the unseen indicator.
func (a *App) reportAgentThreadState() {
	if a.agentSidebar.report == nil {
		return
	}
	t := a.agentSidebar.thread
	status := a.agentSidebar.statusText
	if !a.agentSidebar.working {
		status = ""
		if a.agentSidebar.unreadTotal() > 0 {
			status = unreadStatusMessage(a.agentSidebar.unreadTotal())
		}
	}
	a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, a.agentSidebar.working, status)
}

// reportAgentThreadUnread publishes the tracked thread's unread state as a
// synthetic completion. Deferred while the assistant is mid-turn — the
// count rides the turn-end report instead, and the turn end is itself the
// completion edge herdr needs.
func (a *App) reportAgentThreadUnread() {
	if a.agentSidebar.reportUnread == nil || a.agentSidebar.working {
		return
	}
	t := a.agentSidebar.thread
	a.agentSidebar.reportUnread(agentSidebarID(t.agentName), t.agentName, t.title, unreadStatusMessage(a.agentSidebar.unreadTotal()))
}

// addUnread, dropUnread, and unreadTotal own the unread set. A reply
// that is deleted stops counting: without that the row keeps claiming
// replies that no longer exist, and a deletion in a workspace the user
// isn't in leaves nothing on screen to reconcile it against.
func (g *agentSidebar) addUnread(ts string) {
	if ts == "" {
		return
	}
	if g.unread == nil {
		g.unread = map[string]struct{}{}
	}
	g.unread[ts] = struct{}{}
}

func (g *agentSidebar) dropUnread(ts string) bool {
	if _, ok := g.unread[ts]; !ok {
		return false
	}
	delete(g.unread, ts)
	return true
}

func (g *agentSidebar) unreadTotal() int { return len(g.unread) }

// unreadStatusMessage renders the sidebar row's status text for n unread
// replies.
func unreadStatusMessage(n int) string {
	if n == 1 {
		return "1 unread reply"
	}
	return fmt.Sprintf("%d unread replies", n)
}

// firstBotMention returns the user ID and display name of the first <@U…>
// mention in text that resolves to a bot or app user.
func (a *App) firstBotMention(text string) (userID, name string, ok bool) {
	for _, id := range mrkdwn.MentionedUserIDs(text) {
		if id, name, ok := a.botUser(id); ok {
			return id, name, true
		}
	}
	return "", "", false
}

// botUser resolves userID to a bot or app user, echoing the ID back so both
// detection paths (mention and root author) return the same shape.
func (a *App) botUser(userID string) (string, string, bool) {
	if userID == "" {
		return "", "", false
	}
	name, isBot, ok := a.agentSidebar.userInfo(userID)
	if !ok || !isBot || name == "" {
		return "", "", false
	}
	return userID, name, true
}

// flattenRootText renders a root message's mrkdwn to whitespace-collapsed
// plain text so no raw wire syntax reaches the sidebar or the tab bar.
func (a *App) flattenRootText(rootText string) string {
	text := messages.FlattenMrkdwn(rootText,
		func(id string) (string, bool) {
			if name, _ := a.userNames.Get(id); name != "" {
				return name, true
			}
			name, _, ok := a.agentSidebar.userInfo(id)
			return name, ok && name != ""
		},
		func(id string) (string, bool) {
			name, ok := a.channelNames[id]
			return name, ok
		})
	return strings.Join(strings.Fields(text), " ")
}

// agentThreadTitle labels the sidebar entry with the thread's home channel
// and a snippet of the flattened root message.
func (a *App) agentThreadTitle(channelID, flat string) string {
	channel := a.channelNames[channelID]
	if channel == "" {
		channel = channelID
	}
	const maxSnippet = 48
	return "#" + channel + " " + truncate.StringWithTail(flat, maxSnippet, "…")
}

// taskIDRe matches tracker-style task ids ("colony-562", "TAIGA-41"). Two+
// letters before the dash keeps single-letter false positives out; short
// hyphenated terms with digits ("sha-256") can still match, and the first
// occurrence wins because task ids conventionally lead the message.
var taskIDRe = regexp.MustCompile(`\b[A-Za-z]{2,}-\d+\b`)

// strayPunctRe matches punctuation left dangling between spaces once the
// task id is lifted out of the middle of a sentence.
var strayPunctRe = regexp.MustCompile(`\s+[:,;.\-]+(\s|$)`)

// maxTabLabel caps the label snippet on both the deterministic and model
// paths; a "[colony-562] " task-id prefix rides on top, so a full label
// can exceed it.
const maxTabLabel = 30

// agentTabLabel derives a short tab name from the flattened root text with
// the agent's own mention already stripped (the tab bar has no room for the
// part every agent thread shares). A task id anywhere in the text is
// hoisted to a leading "[colony-562] " prefix, matching the
// workspace-labeling convention, and removed from the snippet so it doesn't
// appear twice.
func agentTabLabel(flat string) string {
	label := strings.TrimLeft(strings.TrimSpace(flat), ":,;.- ")
	if label == "" {
		label = flat
	}
	if id := taskIDRe.FindString(label); id != "" {
		return withTaskID(id, normalizeTabSnippet(strings.Replace(label, id, "", 1)))
	}
	return truncate.StringWithTail(label, maxTabLabel, "…")
}

// normalizeTabSnippet collapses the gap a removed task id leaves (lifting
// "slk-373" out of "traffic for slk-373, the fix" would strand "traffic
// for , the fix"), trims dangling punctuation, and truncates to
// maxTabLabel. Shared by the deterministic and model label paths so their
// rendering can't drift.
func normalizeTabSnippet(s string) string {
	s = strayPunctRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, ":,;.- ")
	s = strings.TrimRight(s, ":,;- ")
	return truncate.StringWithTail(s, maxTabLabel, "…")
}

// withTaskID renders the "[id] snippet" tab-label shape, "[id]" alone when
// the snippet is empty.
func withTaskID(id, snippet string) string {
	if snippet == "" {
		return "[" + id + "]"
	}
	return "[" + id + "] " + snippet
}

// agentSidebarID derives the sidebar's internal agent id from a bot display
// name. The "slack-" prefix keeps it from colliding with herdr's built-in
// agent kinds (a bare "claude" would fight herdr's own claude detection).
func agentSidebarID(displayName string) string {
	id := strings.ToLower(strings.Join(strings.Fields(displayName), "-"))
	return "slack-" + id
}

// reduceAgentThread forwards ai_assistant_status transitions for the
// tracked agent thread to the sidebar, and re-asserts unread state when
// the herdr focus watcher reports the tab was unfocused. Statuses arrive
// workspace-wide, so everything not matching the tracked thread is
// swallowed here.
var reduceAgentThread reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	switch m := msg.(type) {
	case AssistantStatusMsg:
		t := a.agentSidebar.thread
		if !t.active || !a.threadEventIsOurs(m.TeamID) || m.ChannelID != t.channelID ||
			m.ThreadTS != t.threadTS || m.BotUserID != t.botUserID {
			return nil, true
		}
		working := m.Status != ""
		a.agentSidebar.working = working
		a.agentSidebar.statusText = m.Status
		status := m.Status
		if !working && a.agentSidebar.unreadTotal() > 0 {
			// The turn-end idle report is itself the completion edge, so
			// unread state deferred during the turn lands here.
			status = unreadStatusMessage(a.agentSidebar.unreadTotal())
		}
		a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, working, status)
		return nil, true

	case HerdrTabViewMsg:
		a.herdrTabViewed, a.herdrTabViewKnown = m.Viewed, true
		// Viewing the tab made herdr mark the entry seen even if the
		// thread stayed unread; leaving it is the moment to re-assert.
		if !m.Viewed && a.agentSidebar.thread.active && a.agentSidebar.unreadTotal() > 0 {
			a.reportAgentThreadUnread()
		}
		// Focusing the tab puts the open thread panel on screen. Replies
		// that arrived while the pane was unviewed rendered without a
		// mark (the reply path's viewedness gate), so the refocus is the
		// read event: schedule the same debounced mark a live reply gets.
		// The fire-time gates drop it if the user flicks away or the
		// panel changes inside the debounce window.
		if m.Viewed && a.threadVisible && a.threadPanel.ThreadTS() != "" {
			return a.scheduleThreadMark(a.threadPanel.ChannelID(), a.threadPanel.ThreadTS()), true
		}
		return nil, true

	case HerdrConnectedMsg:
		// A plain state report, never a completion: restoring the row
		// after a reconnect must not light the unseen indicator.
		if a.agentSidebar.thread.active {
			a.reportAgentThreadState()
		}
		return nil, true
	}
	return nil, false
}
