package tablabel

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// Verdict values are herdr's lifecycle-state names, so the caller can
// report one without translating it.
type Verdict string

const (
	VerdictIdle    Verdict = "idle"
	VerdictWorking Verdict = "working"
	VerdictBlocked Verdict = "blocked"
)

// The two ambiguous shapes are two different questions, so each side gets
// its own system prompt: one prompt covering both misreads one side or
// the other (an agent waiting for the user's go as working, or a user's
// "go ahead" as the agent not having started).
//
// The agent-side prompt asks about the thread's work, not about the agent
// that wrote the message: another agent session can relay into the thread
// (a reviewer reporting that the author session will merge), and that
// writer is finished while the thread's work is still in flight.
const workingAgentSystemPrompt = "You watch Slack threads where coding agents work on a task for a user. " +
	"The newest message in the thread is from an agent: the thread's own agent, or another agent session that relays into the thread (a reviewer session, for example). Classify from it whether work for this thread is still in flight. " +
	"w: working: this agent or any other agent or agent session holds a next step and does not wait on the user. Acknowledging an instruction (will do, on it), stating how it will proceed, or mentioning something it will need from the user later, while continuing now, is w. " +
	"A report that also names a next step held by an agent, or work an agent session still has, not the user's (I'll post again at the next merge, the author session has fixes or nits in my review, the author session will run the gates and merge, the last PRs wait for the reviewers' second reads), is w, even when the message says its own part is done and the user has nothing to do, and even when a review or merge is pending. " +
	"A report with no such step and no work left with any agent, or whose only next step is the user's to take when they want (review it, merge it, say if you want X), stays d. " +
	"u: no agent can continue until the user answers in this thread: the message asked the user a direct question, presented options or a plan and waits for approval, or is stuck on something only the user can provide. A message that asks the user nothing is never u. Offering an optional follow-up after the work is finished (say if you want X, let me know if you would like Y) is d, never u: no agent waits for the answer. Noting that a review or merge is pending on the user, or work that continues in another thread, is not u. " +
	"d: done, no agent has a stated next step: a result, a report, an answer or explanation that asks nothing back, or work handed over for the user to review or merge, even if it invites feedback. " +
	"Reply with exactly one letter: w, u, or d."

const workingUserSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the user; the agent reacted to it with an emoji and has not replied yet, so the agent owes a response to anything it asks. " +
	"Judge whether the message asks the agent for anything: a request, a question to answer, a decision, or a go-ahead the agent must act on (merge it, open the PR). " +
	"If it does, the agent has work to do. " +
	"If it only closes the exchange (thanks, approval of finished work, an fyi with no action, a request to stop or wait), the agent has nothing to do. " +
	"Reply with exactly one letter: y if the agent has work to do, n if not."

var (
	agentVerdictLetters = map[byte]Verdict{'w': VerdictWorking, 'u': VerdictBlocked, 'd': VerdictIdle}
	userVerdictLetters  = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
)

// maxWorkingBytes caps the one message a working judgment sends. The cap
// keeps both ends: a long post opens with what the agent did and closes
// with the hand-off ("I'll wait for your go"), and either end can carry
// the verdict.
const maxWorkingBytes = 4000

// Judge reads the thread's newest message alone. For an agent's message
// (fromAgent: the thread's agent, or another agent session relaying into
// the thread) it asks whether the thread's work is in flight with any
// agent, waits on the user, or is done; for a user message the agent has
// acknowledged with a reaction but not answered, it asks whether the
// message gives the agent anything to do, which is never VerdictBlocked.
func (c *Client) Judge(ctx context.Context, message string, fromAgent bool) (Verdict, error) {
	system, letters := workingAgentSystemPrompt, agentVerdictLetters
	if !fromAgent {
		system, letters = workingUserSystemPrompt, userVerdictLetters
	}
	reply, err := c.complete(ctx, system, "Newest message:\n"+clipEnds(message, maxWorkingBytes))
	if err != nil {
		return VerdictIdle, err
	}
	return parseVerdict(reply, letters)
}

// parseVerdict reads the one-letter contract leniently: any completion
// leading with a known letter counts, so "yes" or "d — looks finished"
// still parse.
func parseVerdict(reply string, letters map[byte]Verdict) (Verdict, error) {
	s := strings.ToLower(strings.TrimSpace(reply))
	if s != "" {
		if v, ok := letters[s[0]]; ok {
			return v, nil
		}
	}
	return VerdictIdle, fmt.Errorf("unparseable working verdict %q", reply)
}

// clipEnds keeps the head and tail of s, marking the cut between them.
func clipEnds(s string, max int) string {
	if len(s) <= max {
		return s
	}
	half := max / 2
	tail := s[len(s)-half:]
	for i := 0; i < len(tail) && !utf8.RuneStart(tail[i]); i++ {
		tail = tail[i+1:]
	}
	return clip(s, half) + "\n[…]\n" + tail
}
