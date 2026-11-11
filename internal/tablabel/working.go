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

// Author is who wrote a thread message, as the judge is told it: each
// earlier message starts with its author and ": ", and the newest
// message's author picks the question. The user's Claude Code is a
// session the user runs on their own machine that posts under the user's
// account; it is an agent session relaying into the thread, not the user.
type Author string

const (
	AuthorAgent           Author = "agent"
	AuthorUser            Author = "user"
	AuthorUsersClaudeCode Author = "user's Claude Code"
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
	"The newest message in the thread is from an agent: the thread's own agent, or another agent session that relays into the thread (a reviewer session, or the user's Claude Code, for example). Classify from it whether work for this thread is still in flight. " +
	"The user's Claude Code is a session the user runs on their own machine, which posts under the user's name. The line before the newest message says whether it wrote the newest message, or whether it has posted in this thread, with its most recent post, which tells only whether it still runs in this thread: classify the newest message. " +
	"w: working: this agent or any other agent or agent session holds a next step and does not wait on the user. The user's own Claude Code is no such session until the user starts it: a message that hands it a step (the open item waits for the run from your Claude Code, a steps file or prompt for your Claude Code to run and post back) is u, even when it says the user has nothing to do. Once it has posted in this thread, it is running there, and a step that the message hands it or names as its own is w. A result or answer that the user's Claude Code wrote in the newest message is w too, since the thread's agent has yet to read it and reply, unless it says nothing here is left for any agent (it stopped following the thread, nothing here needs to act). A merge word that is the user's to give when they want is not such a step. Acknowledging an instruction (will do, on it), stating how it will proceed, or mentioning something it will need from the user later, while continuing now (I'll post the PR numbers when they're up, and a merge word for each will then let me merge it), is w. " +
	"A report that also names a next step held by an agent, or work an agent session still has, not the user's (I'll post again at the next merge, the author session has fixes or nits in my review, the author session will run the gates and merge, the last PRs wait for the reviewers' second reads), is w, even when the message says its own part is done and the user has nothing to do, and even when a review or merge is pending. " +
	"A report with no such step and no work left with any agent, or whose only next step is the user's to take when they want (review it, say if you want X), stays d. " +
	"u: no agent can continue until the user answers in this thread: the message asked the user a direct question, presented options or a plan and waits for approval, or is stuck on something only the user can provide. Offering an optional follow-up after the work is finished (say if you want X, let me know if you would like Y) is d, never u: no agent waits for the answer. Noting that a review is pending on the user without asking for it now, or work that continues in another thread, is not u. Announcing that an agent session will start, align with the user or debrief them asks nothing now either: the session holds the next step, so it is w. Asking the user now for the word or the answer that a piece of work waits on (type merge and I will merge it, does your go-ahead include the merge?), or saying again that it still waits for a word they have not typed yet (the PR still waits for your typed merge), is different from both: it is u, not w, even when other work continues, an agent holds another step, or the message says what happens with no answer. A PR that is ready now and waits only for the user's merge word is u as well, even when the message asks nothing and only calls the word theirs (merge word is yours): the merge is not an optional follow-up, and nothing goes on without it. A merge an agent already made leaves nothing open. A word the user already typed is different: when the message says an agent's step goes ahead on it (it merges on your \"ok to merge\" once the review has nothing left), the step is the agent's and nothing waits on the user, so it is w. A word that the message tells the user to type, calls theirs to give, or says the work still waits for, is not typed yet. A word that the message says it needs only once work an agent still has is finished (once the reviews say nothing is left, each PR needs your merge word) is not asked for now: that work holds the next step, so it is w. " +
	"d: done, no agent has a stated next step: a result, a report, an answer or explanation that asks nothing back, or work handed over for the user to review, even if it invites feedback. " +
	"Reply with exactly one letter: w, u, or d. The reply is that letter alone, with no reasoning before or after it."

const workingUserSystemPrompt = "You watch Slack threads where a coding agent works on tasks for a user. " +
	"The newest message in the thread is from the user; the agent reacted to it with an emoji and has not replied yet, so the agent owes a response to anything it asks. " +
	"Judge whether the message asks the agent for anything: a request, a question to answer, a decision, or a go-ahead the agent must act on (merge it, open the PR). " +
	"If it does, the agent has work to do. " +
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
	"The ask is open when no later user message answers it and no later agent message withdraws it (that run isn't needed after all) or reports that piece of work finished. The word itself, yes, no, hold off, or a choice between the options are answers. A question back from the user (to clarify, does this mean X?) is not an answer, and an agent message that answers that question leaves the ask open. " +
	"Reply on one line: first the opening five words of the direct ask, copied from the agent message, in quotes, or the word none; then one letter, y when that ask is still open, n when there is none or it is closed. The reply is that one line alone, with no reasoning before or after it."

// openAskNewestHeading tells the open-ask question what the newest message is
// there for. Under the plain heading the model counted what the newest
// message itself offers ("say if you want the links back") as the open ask,
// whatever the system prompt said. parseOpenAsk catches what still gets by.
const openAskNewestHeading = "Newest message (it asks the user nothing; read it only for whether it withdraws an earlier ask or reports that piece of work finished):\n"

const workingEarlierStepSystemPrompt = "You watch Slack threads where coding agents work on a task for a user. " +
	"The newest message in the thread is from an agent and reads as finished on its own: it gives a result and names no next step for any agent. " +
	"The thread's messages before it come first, oldest first, each marked agent, user, or user's Claude Code (an agent session that posts under the user's name). Decide whether they leave a piece of work with an agent that the newest message does not cover. " +
	"y: you can name a specific piece of work (a PR, a fix, a check) that an earlier agent message says an agent or agent session is doing or will do (I'm fixing those now, I'll ask you for the merge after the review, the author session will push the fixes), and the newest message is about a different piece and says nothing that finishes this one. " +
	"n: in every other case. The newest message covers a piece when it reports that piece finished, merged, opened for review, handed to the user, or stopped at the user's word (on hold, nothing more on it until you say so), or when it says all of the work is finished (both PRs are merged now, all 5 are merged). A message between the two can cover it too. An earlier acknowledgement that names no piece of work of its own (on it, will do, I'll report back here when it's done) never counts, whatever the newest message reports: the newest message is what it promised. An earlier plan for the work that the newest message reports (cloning main and checking now) is covered by that report. A step that is the user's to take (review it, merge it, answer if you want) is never such a piece, and neither is work that the user called off or put on hold, or that an agent will do only after a word the user has not given. " +
	"Reply with exactly one letter: y or n. The reply is that letter alone, with no reasoning before or after it."

var (
	agentVerdictLetters       = map[byte]Verdict{'w': VerdictWorking, 'u': VerdictBlocked, 'd': VerdictIdle}
	userVerdictLetters        = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
	earlierStepVerdictLetters = map[byte]Verdict{'y': VerdictWorking, 'n': VerdictIdle}
)

// maxWorkingBytes caps the newest message a working judgment sends,
// maxEarlierBytes each message sent before it as context, and
// maxUsersClaudeCodePostBytes the most recent post of the user's Claude
// Code. The caps keep both ends: a long post opens with what the agent did
// and closes with the hand-off ("I'll wait for your go"), and either end
// can carry the verdict.
const (
	maxWorkingBytes             = 4000
	maxEarlierBytes             = 1500
	maxUsersClaudeCodePostBytes = 600
)

// Judge classifies the thread as of its newest message, written by author.
// For an agent's message (the thread's agent, or another agent session
// relaying into the thread, the user's Claude Code included) it asks
// whether the thread's work is in flight with any agent, waits on the
// user, or is done. That first call also gets usersClaudeCodePost, the
// most recent post of the user's Claude Code in this thread ("" when it
// has not posted), since the newest message alone cannot say whether that
// session is running. The newest message decides alone, with one
// exception: a message that reads done alone is asked about again with
// earlier (the messages before it, oldest first, each "<author>: text"),
// because one of them can hold an ask the user has not answered (blocked),
// or say another fix is still under way (working). For a user message the
// agent has acknowledged with a reaction but not answered, it asks whether
// the message gives the agent anything to do, which is never
// VerdictBlocked; earlier and usersClaudeCodePost play no part in that.
func (c *Client) Judge(ctx context.Context, message string, earlier []string, author Author, usersClaudeCodePost string) (Verdict, error) {
	clipped := clipEnds(message, maxWorkingBytes)
	newest := "Newest message:\n" + clipped
	if author == AuthorUser {
		reply, err := c.complete(ctx, workingUserSystemPrompt, newest)
		if err != nil {
			return VerdictIdle, err
		}
		return parseVerdict(reply, userVerdictLetters)
	}
	reply, err := c.complete(ctx, workingAgentSystemPrompt, usersClaudeCodeLine(author, usersClaudeCodePost)+"\n\n"+newest)
	if err != nil {
		return VerdictIdle, err
	}
	verdict, err := parseVerdict(reply, agentVerdictLetters)
	if err != nil || verdict != VerdictIdle || len(earlier) == 0 {
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

// usersClaudeCodeLine is what the first agent-side call reads before the
// newest message: whether the user's Claude Code wrote it, or else whether
// that session has posted in this thread, and what it last said. The post
// goes along, and not only whether there is one, because the session can
// say there that it stopped following the thread. When it has not posted,
// the line spells out what follows: "has not posted" alone read a steps
// file handed to it as a step a running session holds, 5 of 5 at medium.
// When it wrote the newest message, the line says that a post in which it
// is stuck until the user acts is u: the system prompt's "a result it wrote
// is w" rule read its post blocked on Docker working 4 of 10 at medium.
// The sentence sits here, where only its own posts reach it: each of nine
// wordings tried in the system prompt cost another row, most often the
// agent's steps file handed to it, alone, read working (up to 10 of 10 at
// medium).
func usersClaudeCodeLine(author Author, usersClaudeCodePost string) string {
	switch {
	case author == AuthorUsersClaudeCode:
		return "The user's Claude Code wrote the newest message, and the thread's agent has not read it yet. If it says there that it cannot go on until the user does something (signs in, fixes their own machine, decides for it), the thread waits on the user: u."
	case usersClaudeCodePost == "":
		return "The user's Claude Code has not posted in this thread, so no session of it runs here yet: a step handed to it waits for the user to start it."
	}
	return "The user's Claude Code has posted in this thread. Its most recent post:\n" + clipEnds(usersClaudeCodePost, maxUsersClaudeCodePostBytes)
}

// parseOpenAsk reads the open-ask reply: the ask's opening words in quotes,
// then y or n. The quote is what makes a y checkable: the model also quotes
// what only the newest message says ("Merge word is yours." y), whatever the
// prompt tells it, so a y counts only when an earlier message from an
// agent or the user's Claude Code holds the quoted words. The open-ask
// prompt does not describe the user's Claude Code label: the label alone
// makes the model count that session's ask, and describing it there made
// the call miss an agent's open merge ask 2 to 3 of 5. A bare none, the
// model leaving off the n that follows it, names no ask.
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
		asker := strings.HasPrefix(m, string(AuthorAgent)+": ") || strings.HasPrefix(m, string(AuthorUsersClaudeCode)+": ")
		if asker && strings.Contains(strings.ToLower(m), quote) {
			return true, nil
		}
	}
	return false, nil
}

// parseVerdict reads the one-letter contract leniently: any completion
// leading with a known letter counts, so "yes" or "d — looks finished"
// still parse, and so does one that reasons first and puts the letter
// alone at the start of its last line ("…a step handed to it is w.\n\nw"),
// which the agent-side call does now and then at medium.
func parseVerdict(reply string, letters map[byte]Verdict) (Verdict, error) {
	s := strings.ToLower(strings.TrimSpace(reply))
	for _, line := range []string{s, strings.TrimSpace(s[strings.LastIndex(s, "\n")+1:])} {
		if line == "" {
			continue
		}
		if v, ok := letters[line[0]]; ok {
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
