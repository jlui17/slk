package ui

import (
	"regexp"

	"github.com/gammons/slk/internal/ui/messages"
)

// todoStampRe matches the trailing "_todos as of 19:04 UTC_" stamp some of
// Claude's Slack todo posts carry. The message's text field flattens
// newlines to spaces, so the stamp is anchored to the end of the text, not
// a line.
var todoStampRe = regexp.MustCompile(`_todos as of \d{1,2}:\d{2} UTC_\s*$`)

// todoPendingRe and todoDoneRe match the todo-item markers: ✱/✳/○/◐/☐
// mark pending or in-progress items and appear only in todo renderings,
// while ✓/✔ also show up as ad-hoc bullets in prose, so done markers only
// count as a todo list when at least two appear. Not every todo post
// carries the stamp (observed live: an all-done list posted without one),
// so markers are a first-class signal, not a fallback.
var (
	todoPendingRe = regexp.MustCompile(`[✱✳○◐☐]`)
	todoDoneRe    = regexp.MustCompile(`[✓✔]`)
)

func isAgentTodoText(text string) bool {
	return todoStampRe.MatchString(text) ||
		todoPendingRe.MatchString(text) ||
		len(todoDoneRe.FindAllString(text, 2)) >= 2
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
	ts       string
	authorID string
	human    bool
	todo     bool
	acked    bool
}

// derivedWorking is the content-derived in-progress signal: a human
// message the agent hasn't reacted to means the agent owes a response,
// and an agent-authored todo post means it is mid-task. Anything else (a
// plain agent reply, an agent-acked human message) reads as idle.
func (g *agentSidebar) derivedWorking() bool {
	l := g.lastMsg
	if l.ts == "" {
		return false
	}
	if l.human {
		return !l.acked
	}
	return l.todo
}

// effectiveWorking combines the assistant's live turn state
// (ai_assistant_status, covering the composing window) with the derived
// signal (covering the gaps between messages). Every gate and report that
// used to read the turn state alone reads this.
func (g *agentSidebar) effectiveWorking() bool { return g.working || g.derivedWorking() }

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
	prev := a.agentSidebar.effectiveWorking()
	last := &a.agentSidebar.lastMsg
	switch {
	case msg.IsEdited:
		// Only an edit of the newest message can change the derived
		// state, and only its todo-ness: author and reactions survive
		// an edit.
		if msg.TS != last.ts {
			return
		}
		last.todo = isAgentTodoText(msg.Text)
	case last.ts != "" && msg.TS <= last.ts:
		// Slack ts strings ("1787780670.859699") order lexically at
		// fixed width, so an echo or out-of-order arrival can't
		// replace a newer message.
		return
	default:
		*last = agentLastMsg{
			ts:       msg.TS,
			authorID: msg.UserID,
			human:    a.agentAuthorIsHuman(msg.UserID),
			todo:     isAgentTodoText(msg.Text),
			acked:    reactionBy(msg.Reactions, t.botUserID),
		}
	}
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
	prev := a.agentSidebar.effectiveWorking()
	a.agentSidebar.lastMsg.acked = !removed
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
	prev := a.agentSidebar.effectiveWorking()
	last.human = false
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
	prev := a.agentSidebar.effectiveWorking()
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
	prev := a.agentSidebar.effectiveWorking()
	a.agentSidebar.lastMsg = agentLastMsg{
		ts:       last.TS,
		authorID: last.UserID,
		human:    a.agentAuthorIsHuman(last.UserID),
		todo:     isAgentTodoText(last.Text),
		acked:    reactionBy(last.Reactions, t.botUserID),
	}
	a.publishAgentThreadDerived(prev)
}

// publishAgentThreadDerived reports the tracked thread's state when a
// derived-state change flipped the effective working value; a same-state
// update publishes nothing, so echoes and reloads can't spam herdr. The
// working→idle report here is a real completion edge, and unread state
// deferred during the run rides it.
func (a *App) publishAgentThreadDerived(prevEffective bool) {
	if a.agentSidebar.report == nil {
		return
	}
	eff := a.agentSidebar.effectiveWorking()
	if eff == prevEffective {
		return
	}
	t := a.agentSidebar.thread
	status := ""
	if eff {
		if a.agentSidebar.working {
			status = a.agentSidebar.statusText
		}
	} else if a.agentSidebar.unreadTotal() > 0 {
		status = unreadStatusMessage(a.agentSidebar.unreadTotal())
	}
	a.agentSidebar.report(agentSidebarID(t.agentName), t.agentName, t.title, eff, status)
}
