package ui

import (
	"regexp"
	"slices"
	"strings"

	"github.com/gammons/slk/internal/ui/messages"
)

// todoStampRe matches the trailing "_todos as of 19:04 UTC_" stamp some of
// Claude's Slack todo posts carry. The message's text field flattens
// newlines to spaces, so the stamp is anchored to the end of the text, not
// a line.
var todoStampRe = regexp.MustCompile(`_todos as of \d{1,2}:\d{2} UTC_\s*$`)

// todoPendingRe matches a pending or in-progress todo item: ✱/✳/○/◐/☐
// starting an item, so a mark named in prose ("a ✓/○ status") is no item.
// Not every todo post carries the stamp, so the marker is a first-class
// signal, not a fallback. Done marks (✓/✔) alone never make a todo post:
// a list with every item done is a finished report, and Claude writes its
// done reports as ✓ checklists.
var todoPendingRe = regexp.MustCompile(`(?:^|\s)[✱✳○◐☐] `)

func isAgentTodoText(text string) bool {
	return todoStampRe.MatchString(text) || todoPendingRe.MatchString(text)
}

// reactionsBy is the emoji any user that by matches reacted to the message
// with.
func reactionsBy(reactions []messages.ReactionItem, by func(userID string) bool) []string {
	var emoji []string
	for _, r := range reactions {
		if slices.ContainsFunc(r.UserIDs, by) {
			emoji = append(emoji, r.Emoji)
		}
	}
	return emoji
}

// isThreadAgent reports whether userID is the tracked thread's agent. A
// thread rooted by the agent's bot_message tracks it by its bot_id (B…),
// but its reactions and assistant statuses carry its bot user's id, and
// slk keeps no map between the two: there any bot user counts, as any bot
// author already counts as the agent's side (agentAuthorIsHuman).
func (a *App) isThreadAgent(userID string) bool {
	botID := a.agentSidebar.thread.botUserID
	if userID == botID {
		return true
	}
	if !strings.HasPrefix(botID, "B") || a.agentSidebar.userInfo == nil {
		return false
	}
	_, isBot, ok := a.agentSidebar.userInfo(userID)
	return ok && isBot
}

// agentLastMsg is what the derived working state knows about the tracked
// thread's newest message. Zero value means no message is known, which
// reads as idle. human is provisional when the author isn't in the user
// cache yet; noteAgentThreadUserResolved corrects it when the resolver
// lands.
type agentLastMsg struct {
	ts       string
	authorID string
	human    bool
	todo     bool
	// agentEmoji is the emoji the agent has on the message.
	agentEmoji []string
	// text is the raw mrkdwn body, kept for the model working judge
	// (agentworking_llm.go), which reads the ambiguous shapes.
	text string
}

// acked reports whether the agent has any reaction on the message. The
// emoji itself is deliberately ignored: the agent acks with whatever fits
// (👍, 🚀, …), so only the reactor identifies an ack.
func (l agentLastMsg) acked() bool {
	return len(l.agentEmoji) > 0
}

// derivedState is the content-derived lifecycle state: a human message
// the agent hasn't reacted to means the agent owes a response. Every other
// message — a plain agent reply, an agent todo post, an agent-acked human
// message — takes the model verdict when one has landed for exactly this
// state. While that verdict is in flight it reads what the request set
// (working for a new message, see maybeJudgeAgentWorking). With nothing
// asked, or once the request failed, an agent todo post reads working (a
// todo list means work goes on) and anything else reads idle.
//
// The todo rule yields to any verdict, idle included: a todo post can ask
// the user for a word ("○ Your merge word") or report finished work, and
// the judge reads which (Justin's decision of 2026-11-10).
func (g *agentSidebar) derivedState() AgentState {
	l := g.lastMsg
	if l.ts == "" {
		return AgentIdle
	}
	if l.human && !l.acked() {
		return AgentWorking
	}
	switch workingJudgeKey(l) {
	case g.workingJudge.judgedKey:
		return g.workingJudge.state
	case g.workingJudge.requestedKey:
		return g.workingJudge.inFlightState
	}
	if !l.human && l.todo {
		return AgentWorking
	}
	return AgentIdle
}

// effectiveState combines the assistant's live turn state
// (ai_assistant_status, covering the composing window) with the derived
// signal (covering the gaps between messages). Every gate and report that
// used to read the turn state alone reads this.
func (g *agentSidebar) effectiveState() AgentState {
	if g.working {
		return AgentWorking
	}
	return g.derivedState()
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
	if !a.trackedThreadIn(teamID, channelID) {
		return
	}
	if msg.ThreadTS != t.threadTS && msg.TS != t.threadTS {
		return
	}
	prev := a.agentSidebar.effectiveState()
	last := &a.agentSidebar.lastMsg
	inFlight := AgentWorking
	switch {
	case msg.IsEdited:
		// Only an edit of the newest message can change the derived
		// state — its todo-ness and its judged text: author and
		// reactions survive an edit. The text is part of the judge key,
		// so new text is a new question and gets re-asked, and whatever
		// answered the old text goes stale by its key. Until the new
		// answer lands the thread holds what it read before the edit,
		// so the edit publishes no edge of its own. An edit that leaves
		// the text alone (a link unfurl) keeps the key: the standing
		// verdict or the request in flight still answers it.
		if msg.TS != last.ts {
			return
		}
		inFlight = a.agentSidebar.derivedState()
		last.todo = isAgentTodoText(msg.Text)
		last.text = msg.Text
	case last.ts != "" && msg.TS <= last.ts:
		// Slack ts strings ("1787780670.859699") order lexically at
		// fixed width, so an echo or out-of-order arrival can't
		// replace a newer message.
		return
	default:
		if last.ts != "" {
			earlier := append(a.agentSidebar.earlierMsgs, *last)
			a.agentSidebar.earlierMsgs = earlier[max(0, len(earlier)-maxJudgeEarlierMsgs):]
		}
		*last = a.agentLastMsgFrom(msg)
	}
	a.maybeJudgeAgentWorking(inFlight)
	a.publishAgentThreadDerived(prev)
}

// agentLastMsgFrom is msg as the tracked thread's newest message.
func (a *App) agentLastMsgFrom(msg messages.MessageItem) agentLastMsg {
	return agentLastMsg{
		ts:         msg.TS,
		authorID:   msg.UserID,
		human:      a.agentAuthorIsHuman(msg.UserID),
		todo:       isAgentTodoText(msg.Text),
		text:       msg.Text,
		agentEmoji: reactionsBy(msg.Reactions, a.isThreadAgent),
	}
}

// noteAgentThreadReaction applies a reaction change to the derived state:
// the agent reacting to the newest message — any emoji — is its ack.
func (a *App) noteAgentThreadReaction(teamID, channelID, ts, userID, emoji string, removed bool) {
	if !a.trackedThreadIn(teamID, channelID) || ts != a.agentSidebar.lastMsg.ts || !a.isThreadAgent(userID) {
		return
	}
	prev := a.agentSidebar.effectiveState()
	last := &a.agentSidebar.lastMsg
	last.agentEmoji = slices.DeleteFunc(last.agentEmoji, func(e string) bool { return e == emoji })
	if !removed {
		last.agentEmoji = append(last.agentEmoji, emoji)
	}
	a.maybeJudgeAgentWorking(AgentWorking)
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
	a.maybeJudgeAgentWorking(AgentWorking)
	a.publishAgentThreadDerived(prev)
}

// noteAgentThreadDeleted forgets the newest message when it is retracted.
// The message before it is kept only as judge context (earlierMsgs), so the
// state reads idle until the next reply or panel snapshot re-establishes it.
func (a *App) noteAgentThreadDeleted(teamID, channelID, ts string) {
	if !a.trackedThreadIn(teamID, channelID) || ts != a.agentSidebar.lastMsg.ts {
		return
	}
	prev := a.agentSidebar.effectiveState()
	a.agentSidebar.lastMsg = agentLastMsg{}
	a.publishAgentThreadDerived(prev)
}

// snapshotAgentThreadLast re-derives the last-message state from the
// thread panel's authoritative content — the fetch path carries reactions,
// so an ack that happened while slk wasn't running is seen here.
//
// nil replies is the root painted alone before the replies load (a thread
// with no replies loads as an empty list): that paint has not read the
// thread, and the user's root on top would read working, so a finished
// reply arriving with the load would publish a false completion.
func (a *App) snapshotAgentThreadLast(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	if replies == nil || !a.tracksThread("", channelID, threadTS) {
		return
	}
	thread := append([]messages.MessageItem{parent}, replies...)
	newest := len(thread) - 1
	var earlier []agentLastMsg
	for _, m := range thread[max(0, newest-maxJudgeEarlierMsgs):newest] {
		earlier = append(earlier, a.agentLastMsgFrom(m))
	}
	prev := a.agentSidebar.effectiveState()
	// A snapshot is a re-read, not news: a request it fires holds what the
	// thread read before, so opening (or restarting onto) a thread whose
	// agent finished long ago can't read working and then complete.
	held := a.agentSidebar.derivedState()
	a.agentSidebar.lastMsg = a.agentLastMsgFrom(thread[newest])
	a.agentSidebar.earlierMsgs = earlier
	a.maybeJudgeAgentWorking(held)
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
