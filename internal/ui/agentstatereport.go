package ui

// AgentStateReport is one entry of the agent-state log: the state slk
// shows for the tracked thread, the newest message it was derived from
// (MessageTS is empty when none is known) and the rule that decided it.
// The Judge* fields, MessageTextHash and Error are set only when the model
// judge answered for that message. The log exists so a wrong status can be
// studied afterwards against the cached messages.
type AgentStateReport struct {
	TeamID          string
	ChannelID       string
	ThreadTS        string
	MessageTS       string
	FromAgent       bool
	State           AgentState
	Source          AgentStateSource
	JudgeReply      string
	JudgeModel      string
	JudgePromptHash string
	MessageTextHash string
	Error           string
}

// AgentStateRecorder persists one AgentStateReport. It is called from the
// update loop, so the implementation must not block on storage.
type AgentStateRecorder func(AgentStateReport)

// SetAgentStateRecorder installs the agent-state log. Unset, nothing is
// recorded.
func (a *App) SetAgentStateRecorder(rec AgentStateRecorder) {
	a.agentSidebar.recordState = rec
}

// recordAgentState logs the tracked thread's current state.
// reportAgentThreadState calls it for every state that goes to herdr; the
// verdict reducer calls it for a judge answer that changed nothing herdr
// shows.
func (a *App) recordAgentState() {
	g := &a.agentSidebar
	if g.recordState == nil {
		return
	}
	state, source := g.effective()
	r := AgentStateReport{
		TeamID:    g.thread.teamID,
		ChannelID: g.thread.channelID,
		ThreadTS:  g.thread.threadTS,
		MessageTS: g.lastMsg.ts,
		FromAgent: g.lastMsg.ts != "" && !g.lastMsg.human,
		State:     state,
		Source:    source,
	}
	// The key check covers a standing verdict whose answer was since
	// replaced by a newer message's: the state still holds, its details
	// are gone.
	if answer := g.workingJudge.answer; (source == SourceJudge || source == SourceJudgeError) &&
		answer.Key == workingJudgeKey(g.lastMsg) {
		r.JudgeReply = answer.Reply
		r.JudgeModel = answer.Model
		r.JudgePromptHash = answer.PromptHash
		r.MessageTextHash = answer.TextHash
		r.Error = answer.Err
	}
	g.recordState(r)
}
