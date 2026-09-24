// The model tab label: judge the tracked agent thread's task id and name
// its work from the thread transcript. One request path serves both
// triggers: the automatic one when the thread opens (agentthread_llm.go),
// which fires once, and the :retitle command here, for a thread that has
// drifted since (brainstorm to design to implementation, a task id filed
// mid-thread).
package ui

import (
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

	"github.com/gammons/slk/internal/ui/messages"
)

// AgentTabRelabelFunc requests a model-judged task id and label from a
// thread transcript. fallbackTaskID is echoed into the result (see
// AgentTabRelabelMsg). Answers with an AgentTabRelabelMsg into the program
// loop, or nothing on failure, leaving the current label standing.
type AgentTabRelabelFunc func(teamID, channelID, threadTS, transcript, fallbackTaskID string)

// AgentTabRelabelMsg carries a model label result back into the program
// loop. TaskID is the model's judgment of which task the thread is about;
// empty means it judged the thread has no id, and FallbackTaskID decides
// what that means. :retitle leaves it empty, so none is authoritative: a
// previously hoisted (possibly wrong) id is dropped, not kept. The
// open-time request sets it to the id hoisted from the root, which then
// survives a none: that request may have seen the root alone, and the root
// id is already on the tab.
type AgentTabRelabelMsg struct {
	TeamID         string
	ChannelID      string
	ThreadTS       string
	TaskID         string
	FallbackTaskID string
	Label          string
}

// reduceAgentTabRelabel lands a model label result on the tab, unless the
// tracked thread moved on while the request was in flight.
var reduceAgentTabRelabel reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(AgentTabRelabelMsg)
	if !ok {
		return nil, false
	}
	t := a.agentSidebar.thread
	if !t.active || a.agentSidebar.nameTab == nil || m.TeamID != t.teamID ||
		m.ChannelID != t.channelID || m.ThreadTS != t.threadTS {
		return nil, true
	}
	id := m.TaskID
	if id == "" {
		id = m.FallbackTaskID
	}
	label := sanitizeModelLabel(m.Label, id)
	if m.TaskID == "" && label == "" {
		return nil, true
	}
	if id != "" {
		label = withTaskID(id, label)
	}
	a.agentSidebar.nameTab(label)
	return nil, true
}

// SetAgentTabRelabeler installs the model-label generator. Unset (outside a
// herdr pane, no API key, or the feature not configured), tab naming stays
// purely deterministic and :retitle reports the feature unconfigured.
func (a *App) SetAgentTabRelabeler(gen AgentTabRelabelFunc) {
	a.agentSidebar.relabelGen = gen
}

func init() { commands["retitle"] = cmdRetitle }

// maxRetitleTranscript caps what a label request sends — sized to fit whole
// threads (400KB ≈ 100K tokens, half of claude-haiku-4-5's window), with
// per-message caps so one pasted log can't crowd out the rest. On
// overflow the newest replies survive; the root always rides.
const (
	maxRetitleTranscript = 400000
	maxRetitleRoot       = 2000
	maxRetitleReply      = 1000
)

// cmdRetitle sends the tracked agent thread's whole transcript for a
// model-judged task id and label; reduceAgentTabRelabel lands the result.
// The thread panel must be showing the tracked thread: its loaded
// messages are the context source.
func cmdRetitle(a *App, _ []string) tea.Cmd {
	t := a.agentSidebar.thread
	if !t.active || a.agentSidebar.nameTab == nil {
		return toastWithClear(a, "No agent thread tracked", 2*time.Second)
	}
	if a.agentSidebar.relabelGen == nil {
		return toastWithClear(a, "Tab labeling not configured (herdr.tab_name_model + anthropic_api_key)", 2*time.Second)
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
	a.agentSidebar.relabelGen(t.teamID, t.channelID, t.threadTS, transcript, "")
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
	flat := truncate.StringWithTail(stripLinkTargets(a.flattenRootText(text)), uint(max), "…")
	if flat == "" {
		return ""
	}
	if name := a.retitleSpeaker(userID); name != "" {
		return name + ": " + flat
	}
	return flat
}

// markdownLinkRe matches a markdown link an agent wrote into its message
// text; Slack's own <url|label> form is already flattened to its label.
var markdownLinkRe = regexp.MustCompile(`\[([^\]]+)\]\(https?://[^\s)]+\)`)

// stripLinkTargets keeps a markdown link's label and drops every URL. A
// link target names nothing the thread is about, and the model reads ids
// out of one (a workspace subdomain, a PR path) when it can see it.
func stripLinkTargets(flat string) string {
	flat = markdownLinkRe.ReplaceAllString(flat, "$1")
	return strings.Join(strings.Fields(urlRe.ReplaceAllString(flat, "")), " ")
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
