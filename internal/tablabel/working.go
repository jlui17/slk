package tablabel

import (
	"context"
	"fmt"
	"strings"
)

const workingSystemPrompt = "You watch Slack threads where a coding agent works on tasks. " +
	"From the newest message in a thread, judge whether the agent is still working — " +
	"mid-task, or saying it is about to do or check something — or done and idle. " +
	"A user message the agent has acknowledged but not answered counts as working " +
	"when it asks the agent for anything, and idle when it just closes the exchange. " +
	"Reply with exactly one letter: y if the agent is still working, n if not."

// maxWorkingBytes caps the one message a working judgment sends; a judgment
// needs the message's intent, not a pasted log's tail.
const maxWorkingBytes = 4000

// Working asks the model whether the agent is still mid-task, judged from
// the thread's newest message alone. fromAgent says whose message it is:
// the agent's own reply, or a user message the agent has acknowledged with
// a reaction but not answered.
func (c *Client) Working(ctx context.Context, message string, fromAgent bool) (bool, error) {
	user := "Newest message, from the agent:\n"
	if !fromAgent {
		user = "Newest message, from the user (the agent reacted to it with an emoji and has not replied yet):\n"
	}
	reply, err := c.complete(ctx, workingSystemPrompt, user+clip(message, maxWorkingBytes))
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
