// Model-judged working state: the deterministic derived signal
// (agentworking.go) can't read two last-message shapes — a plain non-todo
// agent reply ("let me check that") and a human message the agent only
// acked with a reaction — so those ask the tab-label model for a
// verdict: working, blocked on the user, or idle. The deterministic
// verdicts (human unacked, todo post) never consult it, and until a
// verdict lands the ambiguous states read idle, exactly as they did
// before this existed.
package ui

import tea "charm.land/bubbletea/v2"

// AgentWorkingJudgeFunc requests a model working/blocked/idle verdict for the
// tracked thread's newest message. key identifies the exact state judged
// (message plus who owes what), so the eventual verdict can be dropped if
// the thread moved on. Fire-and-forget: the implementation answers with an
// AgentWorkingVerdictMsg into the program loop, or nothing on failure.
type AgentWorkingJudgeFunc func(teamID, channelID, threadTS, key, message string, fromAgent bool)

// AgentWorkingVerdictMsg carries a working judgment back into the program
// loop.
type AgentWorkingVerdictMsg struct {
	TeamID    string
	ChannelID string
	ThreadTS  string
	Key       string
	State     AgentState
}

// workingJudgeState tracks the model verdict machinery: the key already
// sent (so echoes and panel reloads can't refire the same question) and
// the key the standing verdict answers. Zero value means nothing asked,
// nothing judged.
type workingJudgeState struct {
	requestedKey string
	judgedKey    string
	state        AgentState
}

// workingJudgeKey names the judged state: the message plus which side wrote
// it. The side matters because a bots.info resolution can flip the same ts
// from human to agent, which changes the question being asked.
func workingJudgeKey(l agentLastMsg) string {
	if l.human {
		return l.ts + "|h"
	}
	return l.ts + "|a"
}

// SetAgentWorkingJudge installs the verdict generator. Unset (no herdr
// pane, no API key, feature unconfigured), the derived state is purely
// deterministic and the ambiguous shapes read idle.
func (a *App) SetAgentWorkingJudge(gen AgentWorkingJudgeFunc) {
	a.agentSidebar.judgeGen = gen
}

// maybeJudgeAgentWorking fires a verdict request when the newest message is
// one of the two ambiguous shapes and that exact state hasn't been asked
// about yet. Every lastMsg mutation calls it; the gates make it a no-op
// everywhere the deterministic signal already decides.
func (a *App) maybeJudgeAgentWorking() {
	g := &a.agentSidebar
	t := g.thread
	l := g.lastMsg
	if g.judgeGen == nil || !t.active || l.ts == "" {
		return
	}
	if l.human && !l.acked || !l.human && l.todo {
		return
	}
	key := workingJudgeKey(l)
	if key == g.workingJudge.requestedKey || key == g.workingJudge.judgedKey {
		return
	}
	message := a.flattenRootText(l.text)
	if message == "" {
		return
	}
	g.workingJudge.requestedKey = key
	g.judgeGen(t.teamID, t.channelID, t.threadTS, key, message, !l.human)
}

// reduceAgentWorkingVerdict lands a working judgment on the derived state,
// unless the thread — or its newest message — moved on while the request
// was in flight.
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
	a.agentSidebar.workingJudge.judgedKey = m.Key
	a.agentSidebar.workingJudge.state = m.State
	a.publishAgentThreadDerived(prev)
	return nil, true
}
