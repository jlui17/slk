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
	"w: working: this agent or any other agent or agent session holds a next step and does not wait on the user. The user's own Claude Code is no such session until the user starts it: a message that hands it a step (the open item waits for the run from your Claude Code, a steps file or prompt for your Claude Code to run and post back) is u, even when it says the user has nothing to do. A merge word that is the user's to give when they want is not such a step. Acknowledging an instruction (will do, on it), stating how it will proceed, or mentioning something it will need from the user later, while continuing now, is w. " +
	"A report that also names a next step held by an agent, or work an agent session still has, not the user's (I'll post again at the next merge, the author session has fixes or nits in my review, the author session will run the gates and merge, the last PRs wait for the reviewers' second reads), is w, even when the message says its own part is done and the user has nothing to do, and even when a review or merge is pending. " +
	"A report with no such step and no work left with any agent, or whose only next step is the user's to take when they want (review it, merge it, say if you want X), stays d. " +
	"u: no agent can continue until the user answers in this thread: the message asked the user a direct question, presented options or a plan and waits for approval, or is stuck on something only the user can provide. A message that asks the user nothing is never u. Offering an optional follow-up after the work is finished (say if you want X, let me know if you would like Y) is d, never u: no agent waits for the answer. Noting that a review or merge is pending on the user without asking for it now (the merge word is yours, the merge stays your typed word), or work that continues in another thread, is not u. Announcing that an agent session will start, align with the user or debrief them, or that later work will wait for the user's word, asks nothing now either: the session holds the next step, so it is w. Asking the user now for the word or the answer that a piece of work waits on (type merge and I will merge it, does your go-ahead include the merge?), or saying again that it still waits for a word they have not typed yet (the PR still waits for your typed merge), is different from both: it is u, not w, even when other work continues, an agent holds another step, or the message says what happens with no answer. " +
	"d: done, no agent has a stated next step: a result, a report, an answer or explanation that asks nothing back, or work handed over for the user to review or merge, even if it invites feedback. " +
	"Reply with exactly one letter: w, u, or d. The reply is that letter alone, with no reasoning before or after it."

const workingUserSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the user; the agent reacted to it with an emoji and has not replied yet, so the agent owes a response to anything it asks. " +
	"Judge whether the message asks the agent for anything: a request, a question to answer, a decision, or a go-ahead the agent must act on (merge it, open the PR). " +
	"If it does, the agent has work to do. A result or report that the user's own Claude Code posts for the agent (the user's local Claude Code here: the count is done) gives the agent work too: it reads the result and replies. " +
	"If it only closes the exchange (thanks, approval of finished work, an fyi with no action, a request to stop or wait), the agent has nothing to do. " +
	"Reply with exactly one letter: y if the agent has work to do, n if not. The reply is that letter alone, with no reasoning before or after it."

// The earlier messages get questions of their own, asked only when the
// newest agent message reads d alone: a w or u newest message decides alone.
// First whether an earlier ask to the user is still open, which turns the d
// into u (the thread needs the user, whatever else goes on); if none is,
// whether an earlier message leaves a step with an agent, which turns it
// into w. One prompt that read the newest and the earlier messages together
// could not hold its verdicts. Every wording that turned the real case (one
// PR merged, the message before it still fixing another) to w also moved
// live rows that the newest message decides alone, in both directions, once
// the most common opening (a user's ask, then the agent's "On it. I'll report
// back here") came before them. Narrow y/n questions hold.
const workingOpenAskSystemPrompt = "You watch Slack threads where coding agents work on a task for a user. " +
	"You get the thread's latest messages, oldest first, each marked agent or user, and then the newest message, which is from an agent and reads as finished on its own. Decide whether an agent message before the newest one put a direct ask to the user that is still open. " +
	"A direct ask is a sentence that tells the user that a piece of work waits on them now: a word for them to type (type merge and I will merge it), a question that they must answer before an agent goes on, an approval of a plan, a choice between options. " +
	"These are not direct asks: a request that the user made of the agent, an acknowledgement (on it, I'll report back), an optional offer (say if you want X), a question that states its own default or that the user need not answer (narrow default: only you; you only need to answer if you want it wider), an ask that the agent says it will make later (I'll ask you for the merge after the review), a question that one agent session tells another to put to the user, and anything that only the newest message says. " +
	"The ask is open when no later user message answers it and no later agent message withdraws it or reports that piece of work finished. The word itself, yes, no, hold off, or a choice between the options are answers. A question back from the user (to clarify, does this mean X?) is not an answer, and an agent message that answers that question leaves the ask open. " +
	"Reply on one line: first the opening five words of the direct ask, copied from the agent message, in quotes, or the word none; then one letter, y when that ask is still open, n when there is none or it is closed. The reply is that one line alone, with no reasoning before or after it."

// openAskNewestHeading tells the open-ask question what the newest message is
// there for. Under the plain heading the model counted what the newest
// message itself offers ("say if you want the links back") as the open ask,
// whatever the system prompt said. parseOpenAsk catches what still gets by.
const openAskNewestHeading = "Newest message (it asks the user nothing; read it only for whether it withdraws an earlier ask or reports that piece of work finished):\n"

const workingEarlierStepSystemPrompt = "You watch Slack threads where coding agents work on a task for a user. " +
	"The newest message in the thread is from an agent and reads as finished on its own: it gives a result and names no next step for any agent. " +
	"The thread's messages before it come first, oldest first, each marked agent or user. Decide whether they leave a piece of work with an agent that the newest message does not cover. " +
	"y: you can name a specific piece of work (a PR, a fix, a check) that an earlier agent message says an agent or agent session is doing or will do (I'm fixing those now, I'll ask you for the merge after the review, the author session will push the fixes), and the newest message is about a different piece and says nothing that finishes this one. " +
	"n: in every other case. The newest message covers a piece when it reports that piece finished, merged, opened for review, handed to the user, or stopped at the user's word (on hold, nothing more on it until you say so), or when it says all of the work is finished (both PRs are merged now, all 5 are merged). A message between the two can cover it too. An earlier acknowledgement that names no piece of work of its own (on it, will do, I'll report back here when it's done) never counts, whatever the newest message reports: the newest message is what it promised. An earlier plan for the work that the newest message reports (cloning main and checking now) is covered by that report. A step that is the user's to take (review it, merge it, answer if you want) is never such a piece, and neither is work that the user called off or put on hold, or that an agent will do only after a word the user has not given. " +
	"Reply with exactly one letter: y or n. The reply is that letter alone, with no reasoning before or after it."

var (
	agentVerdictLetters       = map[byte]Verdict{'w': VerdictWorking, 'u': VerdictBlocked, 'd': VerdictIdle}
	userVerdictLetters        = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
	earlierStepVerdictLetters = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
)

// maxWorkingBytes caps the newest message a working judgment sends, and
// maxEarlierBytes each message sent before it as context. The caps keep
// both ends: a long post opens with what the agent did and closes with the
// hand-off ("I'll wait for your go"), and either end can carry the verdict.
const (
	maxWorkingBytes = 4000
	maxEarlierBytes = 1500
)

// Judge classifies the thread as of its newest message. For an agent's
// message (fromAgent: the thread's agent, or another agent session relaying
// into the thread) it asks whether the thread's work is in flight with any
// agent, waits on the user, or is done. The newest message decides alone,
// with one exception: a message that reads done alone is asked about again
// with earlier (the messages before it, oldest first, each "agent: text" or
// "user: text"), because one of them can hold an ask the user has not
// answered (blocked), or say another fix is still under way (working). For a
// user message the agent has acknowledged with a reaction but not answered,
// it asks whether the message gives the agent anything to do, which is never
// VerdictBlocked; earlier plays no part in that.
func (c *Client) Judge(ctx context.Context, message string, earlier []string, fromAgent bool) (Verdict, error) {
	system, letters := workingAgentSystemPrompt, agentVerdictLetters
	if !fromAgent {
		system, letters = workingUserSystemPrompt, userVerdictLetters
	}
	clipped := clipEnds(message, maxWorkingBytes)
	newest := "Newest message:\n" + clipped
	reply, err := c.complete(ctx, system, newest)
	if err != nil {
		return VerdictIdle, err
	}
	verdict, err := parseVerdict(reply, letters)
	if err != nil || !fromAgent || verdict != VerdictIdle || len(earlier) == 0 {
		return verdict, err
	}
	var content strings.Builder
	content.WriteString("Earlier messages, oldest first:\n")
	for _, m := range earlier {
		content.WriteString(clipEnds(m, maxEarlierBytes) + "\n")
	}
	content.WriteString("\n")
	reply, err = c.complete(ctx, workingOpenAskSystemPrompt, content.String()+openAskNewestHeading+clipped)
	if err != nil {
		return VerdictIdle, err
	}
	open, err := parseOpenAsk(reply, earlier)
	if err != nil {
		return VerdictIdle, err
	}
	if open {
		return VerdictBlocked, nil
	}
	reply, err = c.complete(ctx, workingEarlierStepSystemPrompt, content.String()+newest)
	if err != nil {
		return VerdictIdle, err
	}
	return parseVerdict(reply, earlierStepVerdictLetters)
}

// parseOpenAsk reads the open-ask reply: the ask's opening words in quotes,
// then y or n. The quote is what makes a y checkable: the model also quotes
// what only the newest message says ("Merge word is yours." y), whatever the
// prompt tells it, so a y counts only when an earlier agent message holds
// the quoted words. A bare none, the model leaving off the n that follows
// it, names no ask.
func parseOpenAsk(reply string, earlier []string) (bool, error) {
	s := strings.TrimRight(strings.ToLower(strings.TrimSpace(reply)), ".")
	if s == "none" {
		return false, nil
	}
	letter := s[strings.LastIndexAny(s, " \"")+1:]
	if letter != "y" && letter != "n" {
		return false, fmt.Errorf("unparseable open-ask verdict %q", reply)
	}
	first, last := strings.Index(s, `"`), strings.LastIndex(s, `"`)
	if letter == "n" || last <= first {
		return false, nil
	}
	quote := strings.TrimRight(s[first+1:last], ".,:;!?* ")
	if quote == "" {
		return false, nil
	}
	for _, m := range earlier {
		if strings.HasPrefix(m, "agent: ") && strings.Contains(strings.ToLower(m), quote) {
			return true, nil
		}
	}
	return false, nil
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
