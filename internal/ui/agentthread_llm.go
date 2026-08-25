package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

// AgentTabLabelFunc requests a model-generated tab label for the tracked
// agent thread, keyed by the thread's coordinates so the eventual result
// can be matched back against whatever is tracked when it lands. root is
// the thread's root message as flattened plain text with the agent's
// mention stripped — the root only, because the request fires when the
// thread opens, before its replies have loaded. Fire-and-forget: the
// implementation answers with an AgentTabLabelMsg into the program loop,
// or with nothing on failure — the deterministic label already on the tab
// is the fallback.
type AgentTabLabelFunc func(teamID, channelID, threadTS, root string)

// AgentTabLabelMsg carries a model-generated tab label back into the
// program loop.
type AgentTabLabelMsg struct {
	TeamID    string
	ChannelID string
	ThreadTS  string
	Label     string
}

// llmLabelState tracks the tracked thread's one model-label request:
// whether it has fired, and the task id hoisted from the root text at
// request time, re-applied as the label's prefix when the result lands
// (the model is told not to echo ids; the prefix convention is ours).
type llmLabelState struct {
	requested bool
	taskID    string
}

// SetAgentTabLabeler installs the model-label generator. Unset (outside a
// herdr pane, no API key, or the feature not configured), tab naming stays
// purely deterministic.
func (a *App) SetAgentTabLabeler(gen AgentTabLabelFunc) {
	a.agentSidebar.labelGen = gen
}

// maybeRequestAgentTabLabel fires the tracked thread's model-label request
// the first time its root text is available — at tracking for a normal
// open, or on the same-thread refresh that backfills a permalink-opened
// thread's root. flat is the flattened root with the agent mention
// stripped, the same text the deterministic label was derived from.
func (a *App) maybeRequestAgentTabLabel(flat string) {
	if a.agentSidebar.labelGen == nil || a.agentSidebar.nameTab == nil ||
		a.agentSidebar.llmLabel.requested || flat == "" {
		return
	}
	t := a.agentSidebar.thread
	a.agentSidebar.llmLabel = llmLabelState{requested: true, taskID: taskIDRe.FindString(flat)}
	a.agentSidebar.labelGen(t.teamID, t.channelID, t.threadTS, flat)
}

// sanitizeModelLabel normalizes a model completion into tab-label shape:
// first line only, surrounding quotes and an echo of taskID (the id
// hoisted at request time) removed, whitespace collapsed, truncated like
// the deterministic label. Only that known id is stripped — a taskIDRe
// sweep would also eat hyphen-digit terms the model wrote ("utf-8",
// "sha-256"). Empty means unusable — the caller keeps the label already
// on the tab.
func sanitizeModelLabel(raw, taskID string) string {
	s := raw
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`“”‘’")
	if taskID != "" {
		s = strings.Replace(s, taskID, "", 1)
	}
	return normalizeTabSnippet(s)
}

// reduceAgentTabLabel lands a model-generated label on the tab, unless the
// tracked thread moved on while the request was in flight or the label
// sanitizes to nothing.
var reduceAgentTabLabel reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(AgentTabLabelMsg)
	if !ok {
		return nil, false
	}
	t := a.agentSidebar.thread
	if !t.active || a.agentSidebar.nameTab == nil || m.TeamID != t.teamID ||
		m.ChannelID != t.channelID || m.ThreadTS != t.threadTS {
		return nil, true
	}
	label := sanitizeModelLabel(m.Label, a.agentSidebar.llmLabel.taskID)
	if label == "" {
		return nil, true
	}
	if id := a.agentSidebar.llmLabel.taskID; id != "" {
		label = withTaskID(id, label)
	}
	a.agentSidebar.nameTab(label)
	return nil, true
}
