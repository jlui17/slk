package tablabel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/anthropics/anthropic-sdk-go"
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
	"w: working, mid-task or saying it is doing or about to do something next without waiting on the user: acknowledging an instruction (will do, on it), stating how it will proceed, mentioning something it will need from the user later, or asking a question but naming the default it goes with unless told otherwise, while continuing now, is w. " +
	"u: the agent has stopped and cannot continue until the user answers in this thread: it asked the user a direct question, presented options or a plan and is waiting for approval, or is stuck on something only the user can provide. A message that asks the user nothing is never u. Offering an optional follow-up after the work is finished (say if you want X, let me know if you would like Y, want me to also do Z?) is d, never u, even when phrased as a question. Noting that a review or merge is pending on the user, or work that continues in another thread, is not u. " +
	"d: done, nothing pending on the agent: a result, a report, an answer or explanation that asks nothing back, or work handed over for the user to review or merge, even if it invites feedback. " +
	"Reply on one line: the letter w, u, or d first, then a reason of at most 10 words."

const workingUserSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the user, and the agent has not replied to it yet, so the agent owes a response to anything it asks. " +
	"Judge whether the message asks the agent for anything: a request, a question to answer, a decision, or a go-ahead the agent must act on (merge it, open the PR). " +
	"If it does, the agent has work to do. " +
	"If it only closes the exchange (thanks, approval of finished work, an fyi with no action, a request to stop or wait), or is addressed to another person and not to the agent, the agent has nothing to do. " +
	"Reply on one line: the letter first, y if the agent has work to do, n if not, then a reason of at most 10 words."

var (
	agentVerdictLetters = map[byte]Verdict{'w': VerdictWorking, 'u': VerdictBlocked, 'd': VerdictIdle}
	userVerdictLetters  = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
)

// maxWorkingBytes caps the one message a working judgment sends. The cap
// keeps both ends: a long post opens with what the agent did and closes
// with the hand-off ("I'll wait for your go"), and either end can carry
// the verdict.
const maxWorkingBytes = 4000

// Judgment is one Judge answer with what it takes to study it later:
// Reply is the model's raw completion, PromptHash names the system prompt
// used (so answers can be compared across prompt edits) and TextHash the
// exact user text sent (so a later edit of the message is detectable).
// The hashes are set even when Judge errors.
type Judgment struct {
	Verdict    Verdict
	Reply      string
	PromptHash string
	TextHash   string
}

// Judge reads the thread's newest message alone. For the agent's own reply
// (fromAgent) it asks whether the agent is working, needs the user, or is
// done; for a user message the agent has not answered (reacted to or not),
// it asks whether the message gives the agent anything to do, which is
// never VerdictBlocked.
func (c *Client) Judge(ctx context.Context, message string, fromAgent bool) (Judgment, error) {
	system, letters := workingAgentSystemPrompt, agentVerdictLetters
	if !fromAgent {
		system, letters = workingUserSystemPrompt, userVerdictLetters
	}
	text := "Newest message:\n" + clipEnds(message, maxWorkingBytes)
	j := Judgment{Verdict: VerdictIdle, PromptHash: shortHash(system), TextHash: shortHash(text)}
	var err error
	// Temperature 0: the same message should get the same verdict every
	// time it is asked.
	if j.Reply, err = c.complete(ctx, anthropic.Float(0), system, text); err != nil {
		return j, err
	}
	j.Verdict, err = parseVerdict(j.Reply, letters)
	return j, err
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:6])
}

// parseVerdict reads only the leading letter. The reason after it is for
// the agent-state log, which keeps the whole reply; any completion leading
// with a known letter counts, so "yes" or "d — looks finished" parse too.
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
