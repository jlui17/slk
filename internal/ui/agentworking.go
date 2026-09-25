package ui

import (
	"regexp"

	"github.com/gammons/slk/internal/ui/messages"
)

// todoPendingRe matches the markers of a pending or in-progress todo item
// (✱/✳/○/◐/☐), which appear only in the agent's todo renderings. A todo
// post with none left — all ✓ marks, with or without the "_todos as of_"
// stamp — says nothing about what happens next, so it goes to the judge
// like any other agent message.
var todoPendingRe = regexp.MustCompile(`[✱✳○◐☐]`)

func hasPendingTodo(text string) bool {
	return todoPendingRe.MatchString(text)
}

// reactionBy reports whether userID reacted to the message with any emoji.
// The emoji itself is deliberately ignored: the agent acks with whatever
// fits (👍, 🚀, …), so only the reactor identifies an ack.
func reactionBy(reactions []messages.ReactionItem, userID string) bool {
	for _, r := range reactions {
		for _, id := range r.UserIDs {
			if id == userID {
				return true
			}
		}
	}
	return false
}

// agentLastMsg is what the derived working state knows about the tracked
// thread's newest message. Zero value means no message is known, which
// reads as idle. human is provisional when the author isn't in the user
// cache yet; noteAgentThreadUserResolved corrects it when the resolver
// lands.
type agentLastMsg struct {
	ts          string
	authorID    string
	human       bool
	pendingTodo bool
	acked       bool
	// text is the mrkdwn body with its lines (workingJudgeSource), kept for
	// the model working judge (agentworking_llm.go), which judges from the
	// newest message alone.
	text string
}

// AgentStateSource names the rule that decided a state, for the
// agent-state log (agentstatereport.go).
type AgentStateSource string

const (
	SourceAssistantStatus AgentStateSource = "assistant_status"
	SourceNoMessage       AgentStateSource = "no_message"
	SourceUnackedHuman    AgentStateSource = "unacked_human"
	SourceTodoPost        AgentStateSource = "todo_post"
	SourceJudge           AgentStateSource = "judge"
	SourceJudgePending    AgentStateSource = "judge_pending"
	SourceJudgeError      AgentStateSource = "judge_error"
)

// derived is the content-derived lifecycle state and the rule that
// decided it. An agent-authored todo post with items still open means it is
// mid-task; every other message takes the model verdict once one has landed
// for exactly this state. Until then a human message the agent hasn't
// reacted to reads working (the agent owes a response), and the rest read
// idle.
func (g *agentSidebar) derived() (AgentState, AgentStateSource) {
	l := g.lastMsg
	if l.ts == "" {
		return AgentIdle, SourceNoMessage
	}
	if !l.human && l.pendingTodo {
		return AgentWorking, SourceTodoPost
	}
	key := workingJudgeKey(l)
	if key == g.workingJudge.judgedKey {
		return g.workingJudge.state, SourceJudge
	}
	if l.human && !l.acked {
		return AgentWorking, SourceUnackedHuman
	}
	if key == g.workingJudge.failedKey {
		return AgentIdle, SourceJudgeError
	}
	return AgentIdle, SourceJudgePending
}

// effective combines the assistant's live turn state
// (ai_assistant_status, covering the composing window) with the derived
// signal (covering the gaps between messages).
func (g *agentSidebar) effective() (AgentState, AgentStateSource) {
	if g.working {
		return AgentWorking, SourceAssistantStatus
	}
	return g.derived()
}

// effectiveState is what every gate and report that used to read the turn
// state alone reads.
func (g *agentSidebar) effectiveState() AgentState {
	state, _ := g.effective()
	return state
}

// statusFor is the row's status text for state: the live turn's text
// while a turn is composing, otherwise the unread count when there is one.
func (g *agentSidebar) statusFor(state AgentState) string {
	if state == AgentWorking {
		if g.working {
			return g.statusText
		}
		return ""
	}
	if n := g.unreadTotal(); n > 0 {
		return unreadStatusMessage(n)
	}
	return ""
}

// agentAuthorIsHuman classifies a message author for the derived state:
// the tracked bot user and anything the user cache marks as a bot (a
// bot_message's bot_id author included, once bots.info resolves it) are
// the agent's side; everything else — any human, not just the current
// user — counts as someone the agent owes a reply.
func (a *App) agentAuthorIsHuman(authorID string) bool {
	// No author at all reads as the agent's side: a human classification
	// would latch "working" with no reply ever able to clear it.
	if authorID == "" || authorID == a.agentSidebar.thread.botUserID {
		return false
	}
	if a.agentSidebar.userInfo == nil {
		return true
	}
	_, isBot, ok := a.agentSidebar.userInfo(authorID)
	return !ok || !isBot
}

// noteAgentThreadActivity tracks the tracked thread's newest message for
// the derived working state. Like noteAgentThreadReply it runs ahead of
// the new-message reducer's filtering to see background workspaces, but
// unlike it, it wants every author and edit echoes too: who spoke last,
// and what they said, is exactly the derived state.
func (a *App) noteAgentThreadActivity(teamID, channelID string, msg messages.MessageItem) {
	t := a.agentSidebar.thread
	if !t.active || !a.threadEventIsOurs(teamID) || channelID != t.channelID {
		return
	}
	if msg.ThreadTS != t.threadTS && msg.TS != t.threadTS {
		return
	}
	prev := a.agentSidebar.effectiveState()
	last := &a.agentSidebar.lastMsg
	switch {
	case msg.IsEdited:
		// Only an edit of the newest message can change the derived
		// state — its pending todos and its judged text: author and
		// reactions survive an edit, but a standing model verdict
		// answered the old text. The new text is a new judge key, so
		// that verdict no longer applies and the message is re-asked.
		if msg.TS != last.ts {
			return
		}
		last.pendingTodo = hasPendingTodo(msg.Text)
		last.text = workingJudgeSource(msg)
	case last.ts != "" && msg.TS <= last.ts:
		// Slack ts strings ("1787780670.859699") order lexically at
		// fixed width, so an echo or out-of-order arrival can't
		// replace a newer message.
		return
	default:
		*last = agentLastMsg{
			ts:          msg.TS,
			authorID:    msg.UserID,
			human:       a.agentAuthorIsHuman(msg.UserID),
			pendingTodo: hasPendingTodo(msg.Text),
			acked:       reactionBy(msg.Reactions, t.botUserID),
			text:        workingJudgeSource(msg),
		}
	}
	a.maybeJudgeAgentWorking()
	a.publishAgentThreadDerived(prev)
}

// noteAgentThreadReaction applies a reaction change to the derived state:
// the agent reacting to the newest message — any emoji — is its ack.
func (a *App) noteAgentThreadReaction(teamID, channelID, ts, userID string, removed bool) {
	t := a.agentSidebar.thread
	if !t.active || !a.threadEventIsOurs(teamID) || channelID != t.channelID ||
		ts != a.agentSidebar.lastMsg.ts || userID != t.botUserID {
		return
	}
	prev := a.agentSidebar.effectiveState()
	a.agentSidebar.lastMsg.acked = !removed
	a.maybeJudgeAgentWorking()
	a.publishAgentThreadDerived(prev)
}

// noteAgentThreadUserResolved corrects a provisional human classification:
// a bot_message author is a bare bot_id the user cache can't answer for
// until bots.info lands, and until then the reply reads as a human's.
func (a *App) noteAgentThreadUserResolved(teamID, userID string, isBot bool) {
	t := a.agentSidebar.thread
	last := &a.agentSidebar.lastMsg
	if !t.active || !a.threadEventIsOurs(teamID) || !isBot ||
		userID != last.authorID || !last.human {
		return
	}
	prev := a.agentSidebar.effectiveState()
	last.human = false
	a.maybeJudgeAgentWorking()
	a.publishAgentThreadDerived(prev)
}

// noteAgentThreadDeleted forgets the newest message when it is retracted.
// The message before it isn't tracked, so the state reads idle until the
// next reply or panel snapshot re-establishes it.
func (a *App) noteAgentThreadDeleted(teamID, channelID, ts string) {
	t := a.agentSidebar.thread
	if !t.active || !a.threadEventIsOurs(teamID) || channelID != t.channelID ||
		ts != a.agentSidebar.lastMsg.ts {
		return
	}
	prev := a.agentSidebar.effectiveState()
	a.agentSidebar.lastMsg = agentLastMsg{}
	a.publishAgentThreadDerived(prev)
}

// snapshotAgentThreadLast re-derives the last-message state from the
// thread panel's authoritative content — the fetch path carries reactions,
// so an ack that happened while slk wasn't running is seen here.
func (a *App) snapshotAgentThreadLast(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	if !a.tracksThread("", channelID, threadTS) {
		return
	}
	t := a.agentSidebar.thread
	last := parent
	if len(replies) > 0 {
		last = replies[len(replies)-1]
	}
	prev := a.agentSidebar.effectiveState()
	a.agentSidebar.lastMsg = agentLastMsg{
		ts:          last.TS,
		authorID:    last.UserID,
		human:       a.agentAuthorIsHuman(last.UserID),
		pendingTodo: hasPendingTodo(last.Text),
		acked:       reactionBy(last.Reactions, t.botUserID),
		text:        workingJudgeSource(last),
	}
	a.maybeJudgeAgentWorking()
	a.publishAgentThreadDerived(prev)
}

// publishAgentThreadDerived reports the tracked thread's state when a
// derived-state change moved the effective state; a same-state update
// publishes nothing, so echoes and reloads can't spam herdr. The
// working→idle report here is a real completion edge, and unread state
// deferred during the run rides it.
func (a *App) publishAgentThreadDerived(prev AgentState) {
	if a.agentSidebar.effectiveState() == prev {
		return
	}
	a.reportAgentThreadState()
}
