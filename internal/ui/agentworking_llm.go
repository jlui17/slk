// Model-judged working state: the deterministic derived signal
// (agentworking.go) reads only one last-message shape for certain, an agent
// todo post with items still open. Everything else asks the tab-label model
// for a verdict: an agent reply ("let me check that", an all-done
// checklist) is working, blocked on the user, or idle; a human message
// gives the agent work to do or does not ("thanks"). Until a verdict lands
// a human message the agent hasn't reacted to reads working and the rest
// read idle, exactly as they did before this existed.
package ui

import tea "charm.land/bubbletea/v2"

// AgentWorkingJudgeFunc requests a model working/blocked/idle verdict for the
// tracked thread's newest message. key identifies the exact state judged
// (message plus who owes what), so the eventual verdict can be dropped if
// the thread moved on. Fire-and-forget: the implementation answers with an
// AgentWorkingVerdictMsg into the program loop, with Err set on failure.
type AgentWorkingJudgeFunc func(teamID, channelID, threadTS, key, message string, fromAgent bool)

// AgentWorkingVerdictMsg carries a working judgment back into the program
// loop. Err is set when the request failed (API error, timeout,
// unparseable reply); State is then meaningless. Reply, Model and the
// hashes only feed the agent-state log (agentstatereport.go).
type AgentWorkingVerdictMsg struct {
	TeamID    string
	ChannelID string
	ThreadTS  string
	Key       string
	State     AgentState
	Err       string

	Reply      string
	Model      string
	PromptHash string
	TextHash   string
}

// workingJudgeState tracks the model verdict machinery: the key already
// sent (so echoes and panel reloads can't refire the same question) and
// the key the standing verdict answers. Zero value means nothing asked,
// nothing judged. failedKey is the key whose request errored, which reads
// idle like an unanswered one; answer is the message that set judgedKey or
// failedKey, kept for the agent-state log.
type workingJudgeState struct {
	requestedKey string
	judgedKey    string
	state        AgentState
	failedKey    string
	answer       AgentWorkingVerdictMsg
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

// maybeJudgeAgentWorking fires a verdict request when the newest message
// isn't a pending todo post and that exact state hasn't been asked about
// yet. Every lastMsg mutation calls it; the agent's ack doesn't change the
// key, so it never re-asks about a human message already judged.
func (a *App) maybeJudgeAgentWorking() {
	g := &a.agentSidebar
	t := g.thread
	l := g.lastMsg
	if g.judgeGen == nil || !t.active || l.ts == "" {
		return
	}
	if !l.human && l.pendingTodo {
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
	j := &a.agentSidebar.workingJudge
	j.answer = m
	if m.Err != "" {
		j.failedKey = m.Key
	} else {
		j.judgedKey = m.Key
		j.state = m.State
	}
	if a.agentSidebar.effectiveState() == prev {
		// Nothing goes to herdr, but the log still wants the judge's
		// answer: an idle verdict and an error leave an idle state idle,
		// and a working verdict leaves an unacked human message working.
		a.recordAgentState()
		return nil, true
	}
	a.reportAgentThreadState()
	return nil, true
}
