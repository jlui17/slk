// The model tab label: judge a thread's task id and name its work from the
// thread transcript. One request path serves both triggers: the automatic
// one when the tracked agent thread opens (agentthread_llm.go), which fires
// once, and the :retitle command here, for whatever thread the panel shows:
// an agent thread that has drifted since (brainstorm to design to
// implementation, a task id filed mid-thread), or any other thread the
// user wants the tab named after. The automatic label never lands over a
// tab label something else set; :retitle is the user asking by name, so its
// result does.
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
// thread transcript. reviewOpen marks a !review-open thread's request, which
// the generator sends with the review-open hints. fallbackTaskID, retitleGen
// and reviewOpen are echoed into the result (see AgentTabRelabelMsg).
// Answers with an AgentTabRelabelMsg into the program loop, or nothing on
// failure, leaving the current label standing.
type AgentTabRelabelFunc func(teamID, channelID, threadTS, transcript, fallbackTaskID string, retitleGen uint64, reviewOpen bool)

// AgentTabRelabelMsg carries a model label result back into the program
// loop. TaskID is the model's judgment of which task the thread is about;
// empty means it judged the thread has no id, and FallbackTaskID decides
// what that means. :retitle leaves it empty, so none is authoritative: a
// previously hoisted (possibly wrong) id is dropped, not kept. The
// open-time request sets it to the id hoisted from the root, which then
// survives a none: that request may have seen the root alone, and the root
// id is already on the tab. RetitleGen numbers a :retitle request, zero
// for the automatic one: a :retitle result lands over a tab label
// something else set, unless a newer :retitle has been requested since
// (see labelStillWanted). ReviewOpen renders the "!review ..." label
// instead (see reviewOpenTabLabel).
type AgentTabRelabelMsg struct {
	TeamID         string
	ChannelID      string
	ThreadTS       string
	TaskID         string
	FallbackTaskID string
	RetitleGen     uint64
	ReviewOpen     bool
	Label          string
}

// reduceAgentTabRelabel lands a model label result on the tab, unless it
// went stale while the request was in flight (see labelStillWanted).
var reduceAgentTabRelabel reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	if m, ok := msg.(agentTabAnnotationMsg); ok {
		a.sendAnnotatedAgentTabLabel(m)
		return nil, true
	}
	m, ok := msg.(AgentTabRelabelMsg)
	if !ok {
		return nil, false
	}
	if a.agentSidebar.nameTab == nil || !a.labelStillWanted(m.TeamID, m.ChannelID, m.ThreadTS, m.RetitleGen) {
		return nil, true
	}
	id := m.TaskID
	if id == "" && !m.ReviewOpen {
		id = m.FallbackTaskID
	}
	label := sanitizeModelLabel(m.Label, id)
	if m.TaskID == "" && label == "" {
		return nil, true
	}
	if m.ReviewOpen {
		label = reviewOpenTabLabel(id, label)
	} else if id != "" {
		label = withTaskID(id, label)
	}
	nameTab := a.agentSidebar.nameTab
	if m.RetitleGen != 0 && a.agentSidebar.forceNameTab != nil {
		nameTab = a.agentSidebar.forceNameTab
	}
	nameTab(label)
	return nil, true
}

// labelStillWanted reports whether a label request's result should still
// land. The automatic request (retitleGen zero) is for the tracked agent
// thread, and stands until another agent thread replaces it. A :retitle
// request stands until a newer :retitle is requested, wherever the panel
// has gone since: the user asked for the tab to be named after that
// thread, and :retitle then esc is the common way to ask.
func (a *App) labelStillWanted(teamID, channelID, threadTS string, retitleGen uint64) bool {
	if retitleGen != 0 {
		return retitleGen == a.agentSidebar.retitleGen
	}
	t := a.agentSidebar.thread
	return t.active && teamID == t.teamID && channelID == t.channelID && threadTS == t.threadTS
}

// SetAgentTabRelabeler installs the model-label generator. Unset (outside a
// herdr pane, no API key, or the feature not configured), tab naming stays
// purely deterministic and :retitle reports the feature unconfigured.
func (a *App) SetAgentTabRelabeler(gen AgentTabRelabelFunc) {
	a.agentSidebar.relabelGen = gen
}

// SetAgentTabForceNamer installs the rename a :retitle result lands
// through (see AgentTabRelabelMsg.RetitleGen). Unset, it lands through the
// guarded rename SetAgentReporter installed.
func (a *App) SetAgentTabForceNamer(forceNameTab AgentTabNameFunc) {
	a.agentSidebar.forceNameTab = forceNameTab
}

func init() { commands["retitle"] = cmdRetitle }

// maxRetitleTranscript caps what a label request sends — sized to fit whole
// threads (400KB ≈ 100K tokens, a tenth of claude-sonnet-5-5's window), with
// per-message caps so one pasted log can't crowd out the rest. On
// overflow the newest replies survive; the root always rides.
const (
	maxRetitleTranscript = 400000
	maxRetitleRoot       = 2000
	maxRetitleReply      = 1000
)

// cmdRetitle sends the whole transcript of the thread open in the thread
// panel, its loaded messages, for a model-judged task id and label;
// reduceAgentTabRelabel lands the result. Any thread will do: on one that
// isn't the tracked agent thread it only names the tab, and tracks
// nothing. Only the tracked thread has a known bot, whose mention the root
// line drops.
func cmdRetitle(a *App, _ []string) tea.Cmd {
	if a.agentSidebar.nameTab == nil {
		return toastWithClear(a, "Tab labels need slk in a herdr pane", 2*time.Second)
	}
	channelID, threadTS := a.threadPanel.ChannelID(), a.threadPanel.ThreadTS()
	if threadTS == "" {
		return toastWithClear(a, "Open a thread first", 2*time.Second)
	}
	if a.agentSidebar.relabelGen == nil {
		return toastWithClear(a, "Tab labeling not configured (herdr.tab_name_model + anthropic_api_key)", 2*time.Second)
	}
	var botUserID string
	if a.tracksThread("", channelID, threadTS) {
		botUserID = a.agentSidebar.thread.botUserID
	}
	parent := a.threadPanel.ParentMsg()
	replies := a.threadPanel.Replies()
	transcript := a.retitleTranscript(parent, replies, botUserID, "")
	if transcript == "" {
		return toastWithClear(a, "Nothing to label yet", 2*time.Second)
	}
	target := labelTarget{teamID: a.activeTeamID, channelID: channelID, threadTS: threadTS, botUserID: botUserID}
	a.agentSidebar.retitleGen++
	return tea.Batch(a.requestAgentTabLabel(target, parent, replies, transcript, "", a.agentSidebar.retitleGen),
		toastWithClear(a, "Re-deriving tab label…", 2*time.Second))
}

// retitleTranscript renders the thread as speaker-prefixed lines: the root
// (botUserID's mention stripped, when set), then extra when set (see
// agentthread_reviewopen.go), then as many of the newest replies as fit
// the budget, in chronological order.
func (a *App) retitleTranscript(parent messages.MessageItem, replies []messages.MessageItem, botUserID, extra string) string {
	root := a.retitleLine(parent.UserID, stripMention(parent.Text, botUserID), maxRetitleRoot)
	budget := maxRetitleTranscript - len(root) - len(extra)
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
	lines := make([]string, 0, len(kept)+2)
	if root != "" {
		lines = append(lines, root)
	}
	if extra != "" {
		lines = append(lines, extra)
	}
	for i := len(kept) - 1; i >= 0; i-- {
		lines = append(lines, kept[i])
	}
	return strings.Join(lines, "\n")
}

// retitleLine flattens one message to "speaker: text", the speaker prefix
// dropped when no cache can name the author.
func (a *App) retitleLine(userID, text string, max int) string {
	return a.speakerRetitleLine(a.retitleSpeaker(userID), text, max)
}

// speakerRetitleLine is retitleLine for an author already named.
func (a *App) speakerRetitleLine(name, text string, max int) string {
	flat := truncate.StringWithTail(stripLinkTargets(a.flattenRootText(text)), uint(max), "…")
	if flat == "" {
		return ""
	}
	if name != "" {
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

// stripSessionLabel drops a trailing " [label]" from a speaker name: a bot's
// per-message username carries a session label, the id-keyed cache keeps the
// first one seen, and the model titles the thread from it.
func stripSessionLabel(name string) string {
	bare, _, found := strings.Cut(name, " [")
	bare = strings.TrimSpace(bare)
	if !found || bare == "" || !strings.HasSuffix(name, "]") {
		return name
	}
	return bare
}

// retitleSpeaker resolves an author name through the same two caches
// flattenRootText resolves mentions with, session label dropped.
func (a *App) retitleSpeaker(userID string) string {
	if userID == "" {
		return ""
	}
	if name, _ := a.userNames.Get(userID); name != "" {
		return stripSessionLabel(name)
	}
	if a.agentSidebar.userInfo != nil {
		if name, _, ok := a.agentSidebar.userInfo(userID); ok {
			return stripSessionLabel(name)
		}
	}
	return ""
}
