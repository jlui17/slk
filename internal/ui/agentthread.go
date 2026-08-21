package ui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/slack/mrkdwn"
	"github.com/gammons/slk/internal/ui/messages"
)

// AgentReportFunc mirrors the open agent thread (a thread whose root message
// mentions, or was written by, a bot user) onto an external agent sidebar.
// agent is the sidebar's
// internal agent id, displayName the human name it shows, title the pane
// title, working the in-progress flag, statusMessage the assistant's
// transient status text ("is thinking…", may be empty). See internal/herdr.
type AgentReportFunc func(agent, displayName, title string, working bool, statusMessage string)

// AgentReleaseFunc removes the agent-sidebar entry published through
// AgentReportFunc.
type AgentReleaseFunc func()

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
	ChannelID string
	ThreadTS  string
	BotUserID string
	Status    string
}

// agentSidebar bundles the herdr agent-sidebar integration: the callbacks
// mirroring the open agent thread onto the sidebar (report/release), the tab
// renamer, the user lookup backing bot-user detection, and the detection
// state itself. The callbacks are nil unless slk runs inside a herdr pane
// (see SetAgentReporter).
type agentSidebar struct {
	report   AgentReportFunc
	release  AgentReleaseFunc
	nameTab  AgentTabNameFunc
	userInfo UserInfoFunc
	thread   agentThreadState
}

// agentThreadState identifies the currently open agent thread — the thread
// visible in the thread panel whose root message mentions, or was written by,
// a bot user. Zero value means no agent thread is open.
type agentThreadState struct {
	active    bool
	channelID string
	threadTS  string
	botUserID string
	agentName string
	title     string
}

// SetAgentReporter installs the agent-sidebar callbacks and the user lookup
// backing bot-user detection. Installed only when slk runs inside a herdr
// pane; unset, agent-thread detection is inert.
func (a *App) SetAgentReporter(report AgentReportFunc, release AgentReleaseFunc, nameTab AgentTabNameFunc, userInfo UserInfoFunc) {
	a.agentSidebar.report = report
	a.agentSidebar.release = release
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
// panel's root message and publishes or removes the sidebar entry
// accordingly. Idempotent: an unchanged detection (same thread, bot, title)
// returns without re-reporting, so replies reloading through setThreadPanel
// can't stomp a live working state back to idle. A nil agentReport (slk not
// in a herdr pane, or integration disabled) makes it a no-op.
func (a *App) updateAgentThread(parent messages.MessageItem, channelID, threadTS string) {
	if a.agentSidebar.report == nil || a.agentSidebar.userInfo == nil {
		return
	}
	botUserID, name, ok := a.firstBotMention(parent.Text)
	if !ok {
		botUserID, name, ok = a.botUser(parent.UserID)
	}
	if !ok {
		a.releaseAgentThread()
		return
	}
	flat := a.flattenRootText(parent.Text)
	next := agentThreadState{
		active:    true,
		channelID: channelID,
		threadTS:  threadTS,
		botUserID: botUserID,
		agentName: name,
		title:     a.agentThreadTitle(channelID, flat),
	}
	if next == a.agentSidebar.thread {
		return
	}
	// herdr keys one sidebar entry per pane, so reporting a different agent
	// id replaces the previous entry — switching between two agents'
	// threads needs no cross-agent release.
	a.agentSidebar.thread = next
	// The initial state is idle: ai_assistant_status is edge-triggered, so
	// a turn already in progress isn't visible until its next event.
	a.agentSidebar.report(agentSidebarID(name), name, next.title, false, "")
	if a.agentSidebar.nameTab != nil {
		// The mention is dropped from the raw text, not trimmed from the
		// flattened string: trimming by rendered name breaks when the
		// in-memory name map and the user cache disagree on the bot's name.
		a.agentSidebar.nameTab(agentTabLabel(a.flattenRootText(stripMention(parent.Text, botUserID))))
	}
}

// stripMention removes every <@userID> mention (bare or labeled) from raw
// mrkdwn text.
func stripMention(text, userID string) string {
	re := regexp.MustCompile(`<@` + regexp.QuoteMeta(userID) + `(\|[^>]*)?>`)
	return re.ReplaceAllLiteralString(text, "")
}

// releaseAgentThread removes the sidebar entry when the agent thread stops
// being the open thread (panel closed, or a non-agent thread replaced it).
func (a *App) releaseAgentThread() {
	if !a.agentSidebar.thread.active {
		return
	}
	a.agentSidebar.thread = agentThreadState{}
	if a.agentSidebar.release != nil {
		a.agentSidebar.release()
	}
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
			if name := a.userNames[id]; name != "" {
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
	const maxLabel = 30
	if id := taskIDRe.FindString(label); id != "" {
		rest := strings.Join(strings.Fields(strings.Replace(label, id, "", 1)), " ")
		rest = strings.TrimLeft(rest, ":,;.- ")
		if rest == "" {
			return "[" + id + "]"
		}
		return "[" + id + "] " + truncate.StringWithTail(rest, maxLabel, "…")
	}
	return truncate.StringWithTail(label, maxLabel, "…")
}

// agentSidebarID derives the sidebar's internal agent id from a bot display
// name. The "slack-" prefix keeps it from colliding with herdr's built-in
// agent kinds (a bare "claude" would fight herdr's own claude detection).
func agentSidebarID(displayName string) string {
	id := strings.ToLower(strings.Join(strings.Fields(displayName), "-"))
	return "slack-" + id
}

// reduceAgentThread forwards ai_assistant_status transitions for the open
// agent thread to the sidebar. Statuses arrive workspace-wide, so everything
// not matching the open agent thread is swallowed here.
var reduceAgentThread reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(AssistantStatusMsg)
	if !ok {
		return nil, false
	}
	t := a.agentSidebar.thread
	if !t.active || m.ChannelID != t.channelID || m.ThreadTS != t.threadTS || m.BotUserID != t.botUserID {
		return nil, true
	}
	a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, m.Status != "", m.Status)
	return nil, true
}
