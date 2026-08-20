package ui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/slack/mrkdwn"
	"github.com/gammons/slk/internal/ui/messages"
)

// agentThreadState identifies the currently open agent thread — the thread
// visible in the thread panel whose root message mentions a bot user. Zero
// value means no agent thread is open.
type agentThreadState struct {
	active    bool
	channelID string
	threadTS  string
	botUserID string
	agentName string
	title     string
}

// updateAgentThread re-evaluates agent-thread detection against the thread
// panel's root message and publishes or removes the sidebar entry
// accordingly. Runs at each open path (thread panel, threads view) and again
// from the permalink backfill, where the root text is only known after the
// fetch. A nil agentReport (slk not in a herdr pane, or integration
// disabled) makes it a no-op.
func (a *App) updateAgentThread(parent messages.MessageItem, channelID, threadTS string) {
	if a.agentReport == nil || a.userInfo == nil {
		return
	}
	botUserID, name, ok := a.firstBotMention(parent.Text)
	if !ok {
		a.releaseAgentThread()
		return
	}
	flat := a.flattenRootText(parent.Text)
	title := a.agentThreadTitle(channelID, flat)
	a.agentThread = agentThreadState{
		active:    true,
		channelID: channelID,
		threadTS:  threadTS,
		botUserID: botUserID,
		agentName: name,
		title:     title,
	}
	// The initial state is idle: ai_assistant_status is edge-triggered, so
	// a turn already in progress isn't visible until its next event.
	a.agentReport(agentSidebarID(name), name, title, false, "")
	if a.agentNameTab != nil {
		// The mention is dropped from the raw text, not trimmed from the
		// flattened string: trimming by rendered name breaks when the
		// in-memory name map and the user cache disagree on the bot's name.
		a.agentNameTab(agentTabLabel(a.flattenRootText(stripMention(parent.Text, botUserID))))
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
	if !a.agentThread.active {
		return
	}
	a.agentThread = agentThreadState{}
	if a.agentRelease != nil {
		a.agentRelease()
	}
}

// firstBotMention returns the user ID and display name of the first <@U…>
// mention in text that resolves to a bot or app user.
func (a *App) firstBotMention(text string) (userID, name string, ok bool) {
	for _, id := range mrkdwn.MentionedUserIDs(text) {
		name, isBot, ok := a.userInfo(id)
		if ok && isBot && name != "" {
			return id, name, true
		}
	}
	return "", "", false
}

// flattenRootText renders a root message's mrkdwn to whitespace-collapsed
// plain text so no raw wire syntax reaches the sidebar or the tab bar.
func (a *App) flattenRootText(rootText string) string {
	text := messages.FlattenMrkdwn(rootText,
		func(id string) (string, bool) {
			if name := a.userNames[id]; name != "" {
				return name, true
			}
			name, _, ok := a.userInfo(id)
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
	t := a.agentThread
	if !t.active || m.ChannelID != t.channelID || m.ThreadTS != t.threadTS || m.BotUserID != t.botUserID {
		return nil, true
	}
	a.agentReport(agentSidebarID(t.agentName), t.agentName, t.title, m.Status != "", m.Status)
	return nil, true
}
