package tablabel

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"
)

// The two ambiguous shapes are two different questions, so each side gets
// its own system prompt: one prompt covering both misreads one side or
// the other (an agent waiting for the user's go as working, or a user's
// "go ahead" as the agent not having started).
const workingAgentSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the agent. " +
	"Judge whether the agent is still working: mid-task, or saying it is doing or about to do something next without waiting on the user. " +
	"It is not working when it has finished, or when the next move is the user's: it asked a question, presented options or a plan for approval, or said it will wait for a go-ahead. " +
	"An unfinished task still counts as not working while the agent waits on the user. " +
	"Mentioning something it will need from the user later, while continuing now, still counts as working. " +
	"A message that neither says what the agent does next nor hands the move to the user, such as a plain answer or report, is not working. " +
	"Reply with exactly one letter: y if the agent is working, n if not."

const workingUserSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the user; the agent reacted to it with an emoji and has not replied yet, so the agent owes a response to anything it asks. " +
	"Judge whether the message asks the agent for anything: a request, a question to answer, a decision, or a go-ahead the agent must act on (merge it, open the PR). " +
	"If it does, the agent has work to do. " +
	"If it only closes the exchange (thanks, approval of finished work, an fyi with no action, a request to stop or wait), the agent has nothing to do. " +
	"Reply with exactly one letter: y if the agent has work to do, n if not."

// maxWorkingBytes caps the one message a working judgment sends. The cap
// keeps both ends: a long post opens with what the agent did and closes
// with the hand-off ("I'll wait for your go"), and either end can carry
// the verdict.
const maxWorkingBytes = 4000

// Working judges the thread's newest message alone. For the agent's own
// reply (fromAgent) it asks whether the agent is still working; for a user
// message the agent has acknowledged with a reaction but not answered, it
// asks whether the message gives the agent anything to do. Both collapse to
// working=true/false for the caller.
func (c *Client) Working(ctx context.Context, message string, fromAgent bool) (bool, error) {
	system := workingAgentSystemPrompt
	if !fromAgent {
		system = workingUserSystemPrompt
	}
	reply, err := c.complete(ctx, system, "Newest message:\n"+clipEnds(message, maxWorkingBytes))
	if err != nil {
		return false, err
	}
	return parseWorkingReply(reply)
}

// parseWorkingReply reads the y/n contract leniently: any completion
// leading with y or n counts, so "yes" or "n — looks finished" still parse.
func parseWorkingReply(reply string) (bool, error) {
	switch s := strings.ToLower(strings.TrimSpace(reply)); {
	case strings.HasPrefix(s, "y"):
		return true, nil
	case strings.HasPrefix(s, "n"):
		return false, nil
	default:
		return false, fmt.Errorf("unparseable working verdict %q", reply)
	}
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
