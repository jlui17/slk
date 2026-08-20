package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

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
	name, ok := a.firstBotMention(parent.Text)
	if !ok {
		a.releaseAgentThread()
		return
	}
	title := a.agentThreadTitle(channelID, parent.Text)
	a.agentThread = agentThreadState{
		active:    true,
		channelID: channelID,
		threadTS:  threadTS,
		agentName: name,
		title:     title,
	}
	// The initial state is idle: ai_assistant_status is edge-triggered, so
	// a turn already in progress isn't visible until its next event.
	a.agentReport(agentSidebarID(name), name, title, false, "")
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

// firstBotMention returns the display name of the first <@U…> mention in
// text that resolves to a bot or app user.
func (a *App) firstBotMention(text string) (string, bool) {
	for _, id := range mrkdwn.MentionedUserIDs(text) {
		name, isBot, ok := a.userInfo(id)
		if ok && isBot && name != "" {
			return name, true
		}
	}
	return "", false
}

// agentThreadTitle labels the sidebar entry with the thread's home channel
// and a snippet of the root message, mentions resolved to @names so the raw
// <@U…> wire syntax never reaches the sidebar.
func (a *App) agentThreadTitle(channelID, rootText string) string {
	channel := a.channelNames[channelID]
	if channel == "" {
		channel = channelID
	}
	text := rootText
	for _, id := range mrkdwn.MentionedUserIDs(rootText) {
		name := a.userNames[id]
		if name == "" {
			name, _, _ = a.userInfo(id)
		}
		if name != "" {
			text = strings.ReplaceAll(text, "<@"+id+">", "@"+name)
		}
	}
	text = strings.Join(strings.Fields(text), " ")
	const maxSnippet = 48
	if r := []rune(text); len(r) > maxSnippet {
		text = string(r[:maxSnippet-1]) + "…"
	}
	return "#" + channel + " " + text
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
	if !t.active || m.ChannelID != t.channelID || m.ThreadTS != t.threadTS {
		return nil, true
	}
	a.agentReport(agentSidebarID(t.agentName), t.agentName, t.title, m.Status != "", m.Status)
	return nil, true
}
