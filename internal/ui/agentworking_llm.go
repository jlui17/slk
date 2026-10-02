// Model-judged working state: the deterministic derived signal
// (agentworking.go) can't read two last-message shapes — a plain non-todo
// agent reply ("let me check that") and a human message the agent only
// acked with a reaction — so those ask the tab-label model for a
// verdict: working, blocked on the user, or idle. The model reads the few
// messages before the newest one too: a reply can read finished alone while
// the one before it says another piece is still under way. An unacked human
// message never consults it. An agent todo post does, and stays working
// unless the verdict is blocked (derivedState). While a verdict is
// in flight a new message reads working, never idle: herdr shows every
// working→idle edge as done, so the idle verdict has to be the only idle
// published for a judged message. With no judge installed, or once a
// request fails, the ambiguous states read idle, exactly as they did
// before this existed.
package ui

import (
	"fmt"
	"hash/fnv"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// AgentWorkingJudgeFunc requests a model working/blocked/idle verdict for the
// tracked thread as of its newest message; earlier is the messages before
// it, oldest first, each as "agent: text" or "user: text". key identifies
// the exact state judged (message plus who owes what), so the eventual
// verdict can be dropped if the thread moved on. Fire-and-forget: the
// implementation always answers with an AgentWorkingVerdictMsg into the
// program loop, with Failed set when the request errored or timed out.
type AgentWorkingJudgeFunc func(teamID, channelID, threadTS, key, message string, earlier []string, fromAgent bool)

// maxJudgeEarlierMsgs is how many messages before the newest one the judge
// reads. A step an agent still holds sits in the last few messages; further
// back mostly finds steps long finished.
const maxJudgeEarlierMsgs = 5

// AgentWorkingVerdictMsg carries a working judgment, or the failure to get
// one, back into the program loop. State means nothing when Failed is set.
type AgentWorkingVerdictMsg struct {
	TeamID    string
	ChannelID string
	ThreadTS  string
	Key       string
	State     AgentState
	Failed    bool
}

// workingJudgeState tracks the model verdict machinery: the key already
// sent (so echoes and panel reloads can't refire the same question), what
// the thread reads while that request is in flight, and the key the
// standing verdict answers. Zero value means nothing asked, nothing judged.
type workingJudgeState struct {
	requestedKey  string
	inFlightState AgentState
	judgedKey     string
	state         AgentState
}

// workingJudgeKey names the judged question: the message, which side wrote
// it, and a hash of its text. The side matters because a bots.info
// resolution can flip the same ts from human to agent, which changes the
// question being asked. The text matters because an edit changes it: a
// request that read the old text must not answer for the new one, while an
// edit that leaves the text alone (a link unfurl) asks nothing new. The
// key is logged, so it carries a hash and never the text. The hash is of
// the raw text, not the flattened form the judge reads: that form is the
// raw text plus name caches that fill in late, and a key that moved when a
// name resolved would drop a standing verdict. The earlier messages the
// judge also reads stay out of the key, so a panel reload that re-seeds
// them asks nothing new.
func workingJudgeKey(l agentLastMsg) string {
	side := "a"
	if l.human {
		side = "h"
	}
	h := fnv.New32a()
	h.Write([]byte(l.text))
	return fmt.Sprintf("%s|%s|%08x", l.ts, side, h.Sum32())
}

// SetAgentWorkingJudge installs the verdict generator. Unset (no herdr
// pane, no API key, feature unconfigured), the derived state is purely
// deterministic and the ambiguous shapes read idle.
func (a *App) SetAgentWorkingJudge(gen AgentWorkingJudgeFunc) {
	a.agentSidebar.judgeGen = gen
}

// judgeMessage is the text the judge is asked about when l is the newest
// message, or "" when l never goes to the judge: none installed, a human
// message the agent has not acked, or nothing to read. An agent todo post
// goes too, for the one verdict that moves it (see derivedState).
func (a *App) judgeMessage(l agentLastMsg) string {
	g := &a.agentSidebar
	if g.judgeGen == nil || !g.thread.active || l.ts == "" {
		return ""
	}
	if l.human && !l.acked {
		return ""
	}
	return a.flattenRootText(l.text)
}

// judgeEarlier is the messages before the newest one as the judge reads
// them: flattened like judgeMessage, and marked with the side that wrote
// each as the user cache knows it now. None go with a user's newest message:
// whether it asks the agent for anything is a question about that message.
func (a *App) judgeEarlier() []string {
	if a.agentSidebar.lastMsg.human {
		return nil
	}
	var earlier []string
	for _, m := range a.agentSidebar.earlierMsgs {
		text := a.flattenRootText(m.text)
		if text == "" {
			continue
		}
		side := "agent"
		if a.agentAuthorIsHuman(m.authorID) {
			side = "user"
		}
		earlier = append(earlier, side+": "+text)
	}
	return earlier
}

// replyAwaitsVerdict reports whether msg, not yet noted as the thread's
// newest message, goes to the judge once it is and takes its state from
// the verdict. Such a reply reads working until the verdict lands and that
// verdict's report is its completion signal, so noteAgentThreadReply leaves
// the synthetic one out. A todo post is not such a reply: no verdict makes
// it idle, so no verdict's report is its completion.
func (a *App) replyAwaitsVerdict(msg messages.MessageItem) bool {
	last := a.agentSidebar.lastMsg
	l := a.agentLastMsgFrom(msg)
	return (last.ts == "" || msg.TS > last.ts) && (l.human || !l.todo) && a.judgeMessage(l) != ""
}

// maybeJudgeAgentWorking fires a verdict request when the newest message is
// one of the two ambiguous shapes and that exact state hasn't been asked
// about yet. Every lastMsg mutation calls it; the gates make it a no-op
// everywhere the deterministic signal already decides. inFlight is what the
// thread reads until the answer lands: working for a new message, and what
// the thread read before for an edit or a panel snapshot, which are not
// news in themselves and must not publish an edge of their own.
func (a *App) maybeJudgeAgentWorking(inFlight AgentState) {
	g := &a.agentSidebar
	t := g.thread
	l := g.lastMsg
	message := a.judgeMessage(l)
	if message == "" {
		return
	}
	key := workingJudgeKey(l)
	if key == g.workingJudge.requestedKey || key == g.workingJudge.judgedKey {
		return
	}
	g.workingJudge.requestedKey = key
	g.workingJudge.inFlightState = inFlight
	g.judgeGen(t.teamID, t.channelID, t.threadTS, key, message, a.judgeEarlier(), !l.human)
}

// reduceAgentWorkingVerdict lands a working judgment on the derived state,
// unless the thread — or its newest message — moved on while the request
// was in flight. A failed request falls back to idle and forgets the ask,
// so a later event for the same message asks again; a stale failure is
// dropped like a stale verdict, which leaves the newer request alone.
var reduceAgentWorkingVerdict reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(AgentWorkingVerdictMsg)
	if !ok {
		return nil, false
	}
	t := a.agentSidebar.thread
	if !t.active || m.TeamID != t.teamID || m.ChannelID != t.channelID ||
		m.ThreadTS != t.threadTS || m.Key != workingJudgeKey(a.agentSidebar.lastMsg) {
		return nil, true
	}
	prev := a.agentSidebar.effectiveState()
	if m.Failed {
		a.agentSidebar.workingJudge = workingJudgeState{}
	} else {
		a.agentSidebar.workingJudge.judgedKey = m.Key
		a.agentSidebar.workingJudge.state = m.State
	}
	a.publishAgentThreadDerived(prev)
	return nil, true
}
