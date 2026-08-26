// The :retitle command: re-derive the tracked agent thread's tab label
// from the thread's recent messages. The open-time label request
// (agentthread_llm.go) fires once, from the root only; a thread that
// drifts — brainstorm to design to implementation, a task id filed
// mid-thread — keeps its stale label until this manual refresh.
package ui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/ui/messages"
)

// AgentTabRelabelFunc requests a model-generated tab label from a
// transcript excerpt of the tracked agent thread's recent messages — the
// :retitle refresh of the open-time AgentTabLabelFunc request. Keyed and
// answered identically: an AgentTabLabelMsg into the program loop, or
// nothing on failure, leaving the current label standing.
type AgentTabRelabelFunc func(teamID, channelID, threadTS, transcript string)

// SetAgentTabRelabeler installs the :retitle generator. Unset, :retitle
// reports the feature unconfigured.
func (a *App) SetAgentTabRelabeler(gen AgentTabRelabelFunc) {
	a.agentSidebar.relabelGen = gen
}

func init() { commands["retitle"] = cmdRetitle }

// maxRetitleTranscript caps the excerpt :retitle sends; recency owns the
// budget (the newest replies survive, the oldest drop). The root always
// rides, capped separately so a long opening can't starve the replies
// the refresh exists to see.
const (
	maxRetitleTranscript = 8000
	maxRetitleRoot       = 2000
	maxRetitleReply      = 1000
)

// cmdRetitle re-derives the tracked agent thread's tab label from its
// recent messages, through the same sanitize → task-id prefix → NameTab
// pipeline as the open-time label. The thread panel must be showing the
// tracked thread: its loaded messages are the context source.
func cmdRetitle(a *App, _ []string) tea.Cmd {
	t := a.agentSidebar.thread
	if !t.active || a.agentSidebar.nameTab == nil {
		return toastWithClear(a, "No agent thread tracked", 2*time.Second)
	}
	if a.agentSidebar.relabelGen == nil {
		return toastWithClear(a, "Tab labeling not configured (herdr.tab_name_model)", 2*time.Second)
	}
	if a.threadPanel.ChannelID() != t.channelID || a.threadPanel.ThreadTS() != t.threadTS {
		return toastWithClear(a, "Open the agent thread first", 2*time.Second)
	}
	parent := a.threadPanel.ParentMsg()
	replies := a.threadPanel.Replies()
	transcript := a.retitleTranscript(parent, replies, t.botUserID)
	if transcript == "" {
		return toastWithClear(a, "Nothing to label yet", 2*time.Second)
	}
	taskID := a.retitleTaskID(parent, replies, t.botUserID)
	if taskID == "" {
		taskID = a.agentSidebar.llmLabel.taskID
	}
	a.agentSidebar.llmLabel = llmLabelState{requested: true, taskID: taskID}
	a.agentSidebar.relabelGen(t.teamID, t.channelID, t.threadTS, transcript)
	return toastWithClear(a, "Re-deriving tab label…", 2*time.Second)
}

// retitleTranscript renders the thread as speaker-prefixed lines: the root
// (bot mention stripped, like every label path), then as many of the
// newest replies as fit the budget, in chronological order.
func (a *App) retitleTranscript(parent messages.MessageItem, replies []messages.MessageItem, botUserID string) string {
	root := a.retitleLine(parent.UserID, stripMention(parent.Text, botUserID), maxRetitleRoot)
	budget := maxRetitleTranscript - len(root)
	var kept []string
	for i := len(replies) - 1; i >= 0; i-- {
		line := a.retitleLine(replies[i].UserID, replies[i].Text, maxRetitleReply)
		if line == "" {
			continue
		}
		if len(line)+1 > budget {
			break
		}
		budget -= len(line) + 1
		kept = append(kept, line)
	}
	lines := make([]string, 0, len(kept)+1)
	if root != "" {
		lines = append(lines, root)
	}
	for i := len(kept) - 1; i >= 0; i-- {
		lines = append(lines, kept[i])
	}
	return strings.Join(lines, "\n")
}

// retitleLine flattens one message to "speaker: text", the speaker prefix
// dropped when no cache can name the author.
func (a *App) retitleLine(userID, text string, max int) string {
	flat := truncate.StringWithTail(a.flattenRootText(text), uint(max), "…")
	if flat == "" {
		return ""
	}
	if name := a.retitleSpeaker(userID); name != "" {
		return name + ": " + flat
	}
	return flat
}

// retitleSpeaker resolves an author name through the same two caches
// flattenRootText resolves mentions with.
func (a *App) retitleSpeaker(userID string) string {
	if userID == "" {
		return ""
	}
	if name, _ := a.userNames.Get(userID); name != "" {
		return name
	}
	if a.agentSidebar.userInfo != nil {
		if name, _, ok := a.agentSidebar.userInfo(userID); ok {
			return name
		}
	}
	return ""
}

// retitleTaskID picks the id the refreshed label hoists: the newest
// message carrying one wins, the root last — a task filed mid-thread
// outranks the id the thread opened with. Every reply is scanned, not
// just the transcript's budget survivors. Empty when no message carries
// an id; the caller keeps the previously hoisted one.
func (a *App) retitleTaskID(parent messages.MessageItem, replies []messages.MessageItem, botUserID string) string {
	for i := len(replies) - 1; i >= 0; i-- {
		if id := hoistTaskID(a.flattenRootText(replies[i].Text)); id != "" {
			return id
		}
	}
	return hoistTaskID(a.flattenRootText(stripMention(parent.Text, botUserID)))
}
