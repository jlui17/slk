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
const workingAgentSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the agent. Classify the agent's state from it. " +
	"w: working, mid-task or saying it is doing or about to do something next without waiting on the user; mentioning something it will need from the user later, while continuing now, is still w. " +
	"u: the agent needs the user to answer in this thread before it can continue: it asked a question, presented options or a plan for approval, said it will wait for a go-ahead, or is stopped on something only the user can provide. Work that continues in another thread is not u. " +
	"d: done, nothing pending on the agent: a result, a report, an answer or explanation that asks nothing back, or work handed over for the user to review or merge, even if it invites feedback. " +
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

// Judge reads the thread's newest message alone. For the agent's own reply
// (fromAgent) it asks whether the agent is working, needs the user, or is
// done; for a user message the agent has acknowledged with a reaction but
// not answered, it asks whether the message gives the agent anything to
// do, which is never VerdictBlocked.
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
