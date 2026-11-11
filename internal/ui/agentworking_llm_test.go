package ui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/tablabel"
	"github.com/gammons/slk/internal/ui/messages"
)

type judgeCall struct {
	teamID, channelID, threadTS, key, message string
	earlier                                   []string
	author                                    tablabel.Author
	usersClaudeCodePost                       string
}

func withWorkingJudge(a *App) *[]judgeCall {
	calls := &[]judgeCall{}
	a.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, earlier []string, author tablabel.Author, usersClaudeCodePost string) {
		*calls = append(*calls, judgeCall{teamID, channelID, threadTS, key, message, earlier, author, usersClaudeCodePost})
	})
	return calls
}

// withHerdrSequence records everything published to herdr in one ordered
// list, the way herdr sees it: a state report is its state, and the
// synthetic completion is the working→idle pair it puts on the wire
// (herdr.Client.ReportUnread). The two recorders newAgentTestApp returns
// keep recording; on their own they can't order a state report against a
// pair.
func withHerdrSequence(a *App, calls *[]agentReportCall, unreads *[]agentUnreadCall) *[]AgentState {
	seq := &[]AgentState{}
	a.SetAgentReporter(
		func(agent, displayName, title string, state AgentState, statusMessage string) {
			*seq = append(*seq, state)
			*calls = append(*calls, agentReportCall{agent, displayName, title, state, state == AgentWorking, statusMessage})
		},
		func(agent, displayName, title, statusMessage string) {
			*seq = append(*seq, AgentWorking, AgentIdle)
			*unreads = append(*unreads, agentUnreadCall{agent, displayName, title, statusMessage})
		},
		a.agentSidebar.nameTab, a.agentSidebar.userInfo,
	)
	return seq
}

func assertHerdrSequence(t *testing.T, seq *[]AgentState, want ...AgentState) {
	t.Helper()
	if !slices.Equal(*seq, want) {
		t.Fatalf("published to herdr: %v, want %v", *seq, want)
	}
}

func agentReply(ts, text string) NewMessageMsg {
	return NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: ts, ThreadTS: "100.0", UserID: "UBOT", Text: text,
	}}
}

func editedAgentReply(ts, text string) NewMessageMsg {
	m := agentReply(ts, text)
	m.Message.IsEdited = true
	return m
}

// verdictFor answers judge call n the way the wiring does: the verdict
// carries the key the request was asked with.
func verdictFor(t *testing.T, calls *[]judgeCall, n int, state AgentState) AgentWorkingVerdictMsg {
	t.Helper()
	if n >= len(*calls) {
		t.Fatalf("no judge call %d to answer: %+v", n, *calls)
	}
	c := (*calls)[n]
	return AgentWorkingVerdictMsg{TeamID: c.teamID, ChannelID: c.channelID, ThreadTS: c.threadTS, Key: c.key, State: state}
}

func failureFor(t *testing.T, calls *[]judgeCall, n int) AgentWorkingVerdictMsg {
	t.Helper()
	m := verdictFor(t, calls, n, "")
	m.Failed = true
	return m
}

func TestPlainAgentReplyAsksJudge(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	// The unacked root is deterministic working: no question to ask.
	if len(*judged) != 0 {
		t.Fatalf("judge fired on deterministic state: %+v", *judged)
	}

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "true, let me go check the workflows",
	}})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call, got %+v", *judged)
	}
	call := (*judged)[0]
	if call.teamID != "T1" || call.channelID != "C1" || call.threadTS != "100.0" ||
		call.author != tablabel.AuthorAgent || !strings.Contains(call.message, "go check") {
		t.Errorf("judge call = %+v", call)
	}
	// The key names the message, the side and a hash of the text. It is
	// logged, so it never carries the text itself.
	if !strings.HasPrefix(call.key, "101.0|a|") || strings.Contains(call.key, "check") {
		t.Errorf("judge key = %q", call.key)
	}
	// Until the verdict lands, the plain reply reads working: an idle here
	// would be a working→idle edge, which herdr shows as done.
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working while verdict in flight, got %+v", got)
	}

	a.Update(verdictFor(t, judged, 0, AgentWorking))
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working after a working verdict, got %+v", got)
	}
}

// The real case: the newest reply reads finished alone, and the reply before
// it says the agent is still fixing something else. The judge gets both.
func TestJudgeReadsTheMessagesBeforeTheNewest(t *testing.T) {
	fixing := agentReply("101.0", "PR <https://example.com/pr/2|#2> is in review, I'm fixing 3 findings now.\nPR #1 merges now.")
	merged := agentReply("102.0", "PR #1 is merged.")
	want := []string{
		// Flattened like the newest message, and marked with its side.
		"user: @Claude take a look at the CI workflows",
		"agent: PR #2 is in review, I'm fixing 3 findings now. PR #1 merges now.",
	}

	t.Run("from a panel snapshot", func(t *testing.T) {
		a, _, _ := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, []messages.MessageItem{fixing.Message, merged.Message})
		if len(*judged) != 1 || !slices.Equal((*judged)[0].earlier, want) {
			t.Fatalf("judge calls = %+v, want one with earlier %q", *judged, want)
		}
	})

	t.Run("from live replies", func(t *testing.T) {
		a, _, _ := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		a.Update(fixing)
		a.Update(merged)
		if len(*judged) != 2 || !slices.Equal((*judged)[1].earlier, want) {
			t.Fatalf("judge calls = %+v, want the second with earlier %q", *judged, want)
		}
		// The context is not part of the question's identity.
		if key := (*judged)[1].key; !strings.HasPrefix(key, "102.0|a|") || strings.Count(key, "|") != 2 {
			t.Errorf("judge key = %q", key)
		}
	})
}

func TestJudgeEarlierMessagesAreBounded(t *testing.T) {
	var replies []messages.MessageItem
	for i := 1; i <= 8; i++ {
		replies = append(replies, agentReply(fmt.Sprintf("10%d.0", i), fmt.Sprintf("step %d", i)).Message)
	}
	want := []string{"agent: step 3", "agent: step 4", "agent: step 5", "agent: step 6", "agent: step 7"}

	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, replies)
	if got := (*judged)[len(*judged)-1].earlier; !slices.Equal(got, want) {
		t.Errorf("earlier after a snapshot = %q, want %q", got, want)
	}

	a, _, _ = newAgentTestApp(t)
	judged = withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	for _, r := range replies {
		a.Update(NewMessageMsg{ChannelID: "C1", Message: r})
	}
	if got := (*judged)[len(*judged)-1].earlier; !slices.Equal(got, want) {
		t.Errorf("earlier after live replies = %q, want %q", got, want)
	}
}

func TestJudgeEarlierMessagesResetOnNewThread(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "On it, checking now."))

	parent := messages.MessageItem{TS: "200.0", Text: "<@UBOT> and the release notes", UserID: "UHUMAN"}
	reply := messages.MessageItem{TS: "201.0", ThreadTS: "200.0", UserID: "UBOT", Text: "Drafted, see the doc."}
	a.setThreadPanel(parent, []messages.MessageItem{reply}, "C1", "200.0")
	call := (*judged)[len(*judged)-1]
	if want := []string{"user: @Claude and the release notes"}; call.threadTS != "200.0" || !slices.Equal(call.earlier, want) {
		t.Errorf("judge call = %+v, want thread 200.0 with earlier %q", call, want)
	}
}

// Whether an acked user message asks the agent for anything is a question
// about that message, so it goes alone.
func TestAckedUserMessageGoesToJudgeAlone(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	ask := messages.MessageItem{TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "thanks!",
		Reactions: []messages.ReactionItem{{Emoji: "+1", Count: 1, UserIDs: []string{"UBOT"}}}}
	openWorkingAgentThread(a, []messages.MessageItem{agentReply("101.0", "Done, all green.").Message, ask})
	if len(*judged) != 1 || (*judged)[0].author != tablabel.AuthorUser || (*judged)[0].earlier != nil {
		t.Errorf("judge calls = %+v, want one user-side call with no earlier messages", *judged)
	}
}

func TestAckReactionAsksJudgeAboutAckedMessage(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call after agent ack, got %+v", *judged)
	}
	call := (*judged)[0]
	if !strings.HasPrefix(call.key, "100.0|h|") || call.author != tablabel.AuthorUser || !strings.Contains(call.message, "CI workflows") {
		t.Errorf("judge call = %+v", call)
	}
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working while verdict in flight, got %+v", got)
	}
	a.Update(verdictFor(t, judged, 0, AgentWorking))
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working after a working verdict on acked ask, got %+v", got)
	}
}

func agentReacts(ts, emoji string) ReactionAddedMsg {
	return ReactionAddedMsg{ChannelID: "C1", MessageTS: ts, UserID: "UBOT", Emoji: emoji}
}

func agentUnreacts(ts, emoji string) ReactionRemovedMsg {
	return ReactionRemovedMsg{ChannelID: "C1", MessageTS: ts, UserID: "UBOT", Emoji: emoji}
}

// userReplyWith is a user's reply carrying reactions, the way a replies
// fetch delivers one the agent reacted to while slk wasn't looking.
func userReplyWith(reactions ...messages.ReactionItem) messages.MessageItem {
	return messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "thanks!", Reactions: reactions}
}

func reactionBy(emoji, userID string) messages.ReactionItem {
	return messages.ReactionItem{Emoji: emoji, Count: 1, UserIDs: []string{userID}}
}

// 👀 from the agent says it is working on the message: nothing to ask.
func TestEyesReactionReadsWorkingWithoutJudge(t *testing.T) {
	t.Run("live reaction", func(t *testing.T) {
		a, reports, unreads := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		seq := withHerdrSequence(a, reports, unreads)
		a.Update(agentReacts("100.0", "eyes"))
		// 👀 wins over any other emoji next to it.
		a.Update(agentReacts("100.0", "+1"))
		// The unanswered root already read working: nothing new for herdr.
		assertHerdrSequence(t, seq)
		if len(*judged) != 0 {
			t.Errorf("judge asked about a message the agent has 👀 on: %+v", *judged)
		}
	})

	for name, reactions := range map[string][]messages.ReactionItem{
		"fetched reaction":              {reactionBy("eyes", "UBOT")},
		"fetched next to another emoji": {reactionBy("+1", "UBOT"), reactionBy("eyes", "UBOT")},
	} {
		t.Run(name, func(t *testing.T) {
			a, reports, _ := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			openWorkingAgentThread(a, []messages.MessageItem{userReplyWith(reactions...)})
			if got := lastReport(t, reports); !got.working {
				t.Errorf("expected working, got %+v", got)
			}
			if len(*judged) != 0 {
				t.Errorf("judge asked about a message the agent has 👀 on: %+v", *judged)
			}
		})
	}
}

// Only the agent's own 👀 counts: a user's 👀 next to the agent's ack leaves
// the message with the judge.
func TestUsersEyesReactionIsNotTheAgents(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, []messages.MessageItem{userReplyWith(reactionBy("eyes", "UHUMAN"), reactionBy("+1", "UBOT"))})
	if len(*judged) != 1 || !strings.HasPrefix((*judged)[0].key, "101.0|h|") {
		t.Errorf("judge calls = %+v, want one about the acked message", *judged)
	}
}

// 👀 added over a standing verdict reads working; taken off, the message is
// the judge's again, and the verdict that stands for it still answers.
func TestEyesReactionOverAVerdict(t *testing.T) {
	for name, open := range map[string]func(*App) string{
		"user message": func(a *App) string {
			openWorkingAgentThread(a, nil)
			a.Update(agentReacts("100.0", "+1"))
			return "100.0"
		},
		"agent message": func(a *App) string {
			openWorkingAgentThread(a, nil)
			a.Update(agentReply("101.0", "Done, all green."))
			return "101.0"
		},
	} {
		t.Run(name, func(t *testing.T) {
			a, reports, unreads := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			ts := open(a)
			a.Update(verdictFor(t, judged, 0, AgentIdle))
			seq := withHerdrSequence(a, reports, unreads)

			a.Update(agentReacts(ts, "eyes"))
			assertHerdrSequence(t, seq, AgentWorking)
			a.Update(agentUnreacts(ts, "eyes"))
			assertHerdrSequence(t, seq, AgentWorking, AgentIdle)
			if len(*judged) != 1 {
				t.Errorf("expected only the first judge call, got %+v", *judged)
			}
		})
	}
}

// A verdict that lands while the agent has 👀 on the message does not move
// it; it answers once the 👀 comes off.
func TestEyesReactionHoldsWorkingOverALandingVerdict(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "Done, all green."))
	a.Update(agentReacts("101.0", "eyes"))
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(verdictFor(t, judged, 0, AgentIdle))
	assertHerdrSequence(t, seq)
	a.Update(agentUnreacts("101.0", "eyes"))
	assertHerdrSequence(t, seq, AgentIdle)
}

func TestEyesReactionRemoved(t *testing.T) {
	t.Run("another agent emoji remains", func(t *testing.T) {
		a, reports, unreads := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		seq := withHerdrSequence(a, reports, unreads)
		a.Update(agentReacts("100.0", "eyes"))
		a.Update(agentReacts("100.0", "+1"))

		a.Update(agentUnreacts("100.0", "eyes"))
		if len(*judged) != 1 || !strings.HasPrefix((*judged)[0].key, "100.0|h|") {
			t.Fatalf("judge calls = %+v, want one about the acked message", *judged)
		}
		// Asked as news, so it reads working until the one idle lands.
		assertHerdrSequence(t, seq)
		a.Update(verdictFor(t, judged, 0, AgentIdle))
		assertHerdrSequence(t, seq, AgentIdle)
	})

	t.Run("no agent emoji remains", func(t *testing.T) {
		a, reports, unreads := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		seq := withHerdrSequence(a, reports, unreads)
		a.Update(agentReacts("100.0", "eyes"))
		a.Update(agentReacts("100.0", "+1"))

		// Taking the other emoji off leaves the 👀: still nothing to ask.
		a.Update(agentUnreacts("100.0", "+1"))
		a.Update(agentUnreacts("100.0", "eyes"))
		assertHerdrSequence(t, seq)
		if len(*judged) != 0 {
			t.Errorf("judge asked about a message the agent has no reaction on: %+v", *judged)
		}
	})
}

func TestStaleVerdictIsDropped(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	// The thread moves on before the verdict lands.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "also this",
	}})
	before := len(*reports)
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	if len(*reports) != before {
		t.Errorf("stale verdict published a report: %+v", (*reports)[before:])
	}
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working from the unacked follow-up, got %+v", got)
	}
}

// The real case: the todo list under a merge ask still has open items.
const todoPostWithAsk = "#2382 is ready for a merge. *Type \"merge 2382\" and I merge it.* ✓ Review clean. ○ Your merge word for #2382. _todos as of 02:38 UTC_"

func TestTodoPostAsksJudge(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", todoPostWithAsk))
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call for the todo post, got %+v", *judged)
	}
	if call := (*judged)[0]; call.author != tablabel.AuthorAgent || !strings.Contains(call.message, "Your merge word") {
		t.Errorf("judge call = %+v", call)
	}
}

// A landed verdict decides a todo post, idle included; only a failed
// request leaves it to the todo rule.
func TestTodoPostTakesItsVerdict(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(*testing.T, *[]judgeCall) AgentWorkingVerdictMsg
		want   AgentState
	}{
		{"blocked verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 0, AgentBlocked) }, AgentBlocked},
		{"working verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 0, AgentWorking) }, AgentWorking},
		{"idle verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 0, AgentIdle) }, AgentIdle},
		{"failed request", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return failureFor(t, c, 0) }, AgentWorking},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, reports, unreads := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			openWorkingAgentThread(a, nil)
			seq := withHerdrSequence(a, reports, unreads)

			// The thread read working before the todo post and reads
			// working while its verdict is in flight: nothing to publish.
			a.Update(agentReply("101.0", todoPostWithAsk))
			assertHerdrSequence(t, seq)
			if got := a.agentSidebar.effectiveState(); got != AgentWorking {
				t.Fatalf("expected working while the verdict is in flight, got %q", got)
			}

			a.Update(tc.answer(t, judged))
			if got := a.agentSidebar.effectiveState(); got != tc.want {
				t.Errorf("state = %q, want %q", got, tc.want)
			}
			if tc.want == AgentWorking {
				assertHerdrSequence(t, seq)
				return
			}
			assertHerdrSequence(t, seq, tc.want)
			if got := lastReport(t, reports); got.status != "1 unread reply" {
				t.Errorf("expected the %s report to carry the unread count, got %+v", tc.want, got)
			}
		})
	}
}

// A todo post goes to the judge and its idle verdict is a completion, so on
// an idle thread it completes once, on that verdict, like a plain reply:
// no synthetic completion when it arrives.
func TestTodoPostOnIdleThreadCompletesOnce(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer func(*testing.T, *[]judgeCall) AgentWorkingVerdictMsg
		want   []AgentState
	}{
		{"blocked verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 1, AgentBlocked) }, []AgentState{AgentWorking, AgentBlocked}},
		{"working verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 1, AgentWorking) }, []AgentState{AgentWorking}},
		{"idle verdict", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return verdictFor(t, c, 1, AgentIdle) }, []AgentState{AgentWorking, AgentIdle}},
		{"failed request", func(t *testing.T, c *[]judgeCall) AgentWorkingVerdictMsg { return failureFor(t, c, 1) }, []AgentState{AgentWorking}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, reports, unreads := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			openWorkingAgentThread(a, nil)
			a.Update(agentReply("101.0", "Done, all green."))
			a.Update(verdictFor(t, judged, 0, AgentIdle))
			seq := withHerdrSequence(a, reports, unreads)

			a.Update(agentReply("102.0", todoPostWithAsk))
			assertHerdrSequence(t, seq, AgentWorking)
			a.Update(tc.answer(t, judged))
			assertHerdrSequence(t, seq, tc.want...)
			if len(*unreads) != 0 {
				t.Errorf("expected no synthetic completion, got %+v", *unreads)
			}
		})
	}
}

// An edit of a todo post (a refreshed stamp) is a new question. Until its
// answer lands the post holds what it read before, like any other edit.
func TestEditOfTodoPostReasksAndHoldsItsState(t *testing.T) {
	for _, held := range []AgentState{AgentBlocked, AgentWorking, AgentIdle} {
		t.Run(string(held), func(t *testing.T) {
			a, reports, unreads := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			openWorkingAgentThread(a, nil)
			a.Update(agentReply("101.0", todoPostWithAsk))
			a.Update(verdictFor(t, judged, 0, held))
			seq := withHerdrSequence(a, reports, unreads)

			a.Update(editedAgentReply("101.0", strings.Replace(todoPostWithAsk, "02:38", "02:50", 1)))
			if len(*judged) != 2 {
				t.Fatalf("expected the edit to re-ask, got %+v", *judged)
			}
			assertHerdrSequence(t, seq)
			if got := a.agentSidebar.effectiveState(); got != held {
				t.Errorf("state while the re-ask is in flight = %q, want %q", got, held)
			}

			// A failed re-ask drops the held verdict: back to the default.
			a.Update(failureFor(t, judged, 1))
			if got := a.agentSidebar.effectiveState(); got != AgentWorking {
				t.Errorf("state after the failed re-ask = %q, want working", got)
			}
		})
	}
}

func TestJudgeNotRefiredForSameState(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	reply := messages.MessageItem{TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check"}
	openWorkingAgentThread(a, []messages.MessageItem{reply})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call from the snapshot, got %+v", *judged)
	}
	// A panel reload re-snapshots the same newest message: same question,
	// not asked again.
	openWorkingAgentThread(a, []messages.MessageItem{reply})
	if len(*judged) != 1 {
		t.Errorf("snapshot reload refired the judge: %+v", *judged)
	}
}

func TestEditOfJudgedMessageReasks(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "Done, see https://example.com/pr/1"))
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	if got := lastReport(t, reports); got.state != AgentIdle {
		t.Fatalf("expected idle after an idle verdict, got %+v", got)
	}
	seq := withHerdrSequence(a, reports, unreads)

	// The edit changes what was judged, so the new text is re-asked. While
	// that request is in flight the thread holds what it read before the
	// edit: a link unfurl is an edit too, and reading working here would
	// publish idle→working→idle, a second done for a message that already
	// had its one.
	a.Update(editedAgentReply("101.0", "Done, see https://example.com/pr/1. Actually, one more fix, on it."))
	if len(*judged) != 2 || !strings.Contains((*judged)[1].message, "one more fix") {
		t.Fatalf("expected a re-ask with the edited text, got %+v", *judged)
	}
	assertHerdrSequence(t, seq)

	a.Update(verdictFor(t, judged, 1, AgentWorking))
	assertHerdrSequence(t, seq, AgentWorking)
}

func TestEditPublishesNoIdleWhileJudgeInFlight(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	seq := withHerdrSequence(a, reports, unreads)

	// The edit lands before the first verdict does.
	a.Update(agentReply("101.0", "On it, checking now."))
	a.Update(editedAgentReply("101.0", "On it, checking the CI logs now."))
	if len(*judged) != 2 {
		t.Fatalf("expected the edit to re-ask, got %+v", *judged)
	}
	assertHerdrSequence(t, seq)

	// A standing working verdict holds through an edit's re-ask as well.
	a.Update(verdictFor(t, judged, 1, AgentWorking))
	a.Update(editedAgentReply("101.0", "On it, checking the CI logs and the runner config now."))
	if len(*judged) != 3 {
		t.Fatalf("expected the second edit to re-ask, got %+v", *judged)
	}
	assertHerdrSequence(t, seq)
	if got := a.agentSidebar.effectiveState(); got != AgentWorking {
		t.Errorf("expected working while the re-ask is in flight, got %q", got)
	}
}

// A link unfurl arrives as message_changed with the text untouched: the
// question the judge was asked has not changed, so nothing is asked again.
func TestUnfurlEditWhileJudgeInFlightAsksNothingNew(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	seq := withHerdrSequence(a, reports, unreads)

	const text = "Relayed, see https://example.slack.com/archives/C1/p101000000"
	a.Update(agentReply("101.0", text))
	a.Update(editedAgentReply("101.0", text))
	if len(*judged) != 1 {
		t.Fatalf("unfurl fired a second request: %+v", *judged)
	}
	assertHerdrSequence(t, seq)

	// The request in flight still answers the message.
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	assertHerdrSequence(t, seq, AgentIdle)
}

func TestUnfurlEditAfterVerdictAsksNothing(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	const text = "Still on it, see https://example.slack.com/archives/C1/p101000000"
	a.Update(agentReply("101.0", text))
	a.Update(verdictFor(t, judged, 0, AgentWorking))
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(editedAgentReply("101.0", text))
	if len(*judged) != 1 {
		t.Fatalf("unfurl re-asked an answered question: %+v", *judged)
	}
	assertHerdrSequence(t, seq)
	if got := a.agentSidebar.effectiveState(); got != AgentWorking {
		t.Errorf("unfurl dropped the standing verdict: %q", got)
	}
}

func TestTextEditInFlightDropsTheOldRequestsAnswer(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(agentReply("101.0", "Done, all green."))
	edit := editedAgentReply("101.0", "Done, all green. Actually, one test is red, fixing it.")
	a.Update(edit)
	if len(*judged) != 2 {
		t.Fatalf("expected the edit to re-ask, got %+v", *judged)
	}

	// The first request answered the old text. Its idle must not show
	// done for a message that now says something else.
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	assertHerdrSequence(t, seq)
	// Nor can its failure end the newer request: the thread still reads
	// working, and a reload does not ask the new question again.
	a.Update(failureFor(t, judged, 0))
	assertHerdrSequence(t, seq)
	if got := a.agentSidebar.effectiveState(); got != AgentWorking {
		t.Errorf("the old request's answer ended the newer request's in-flight state: %q", got)
	}
	openWorkingAgentThread(a, []messages.MessageItem{edit.Message})
	if len(*judged) != 2 {
		t.Errorf("the old request's failure forgot the newer request: %+v", *judged)
	}

	a.Update(verdictFor(t, judged, 1, AgentIdle))
	assertHerdrSequence(t, seq, AgentIdle)
}

func TestNewReplyPublishesNoIdleWhileJudgeInFlight(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "On it, checking now."))
	a.Update(verdictFor(t, judged, 0, AgentWorking))
	seq := withHerdrSequence(a, reports, unreads)

	// The real case: the newest message was judged working, and the next
	// agent message is one the judge also reads as working. herdr must see
	// nothing at all, because any idle in between shows as done.
	a.Update(agentReply("102.0", "Found the cause, writing the fix."))
	assertHerdrSequence(t, seq)
	a.Update(verdictFor(t, judged, 1, AgentWorking))
	assertHerdrSequence(t, seq)
}

func TestIdleVerdictIsTheOneCompletion(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "On it, checking now."))
	a.Update(verdictFor(t, judged, 0, AgentWorking))
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(agentReply("102.0", "Done, all green."))
	assertHerdrSequence(t, seq)
	a.Update(verdictFor(t, judged, 1, AgentIdle))
	// One working→idle edge, carrying the unread count deferred during the
	// run, and no synthetic pair on top of it.
	assertHerdrSequence(t, seq, AgentIdle)
	if got := lastReport(t, reports); got.status != "2 unread replies" {
		t.Errorf("expected the completion to carry the unread count, got %+v", got)
	}
}

func TestReplyOnIdleThreadCompletesOnceOnIdleVerdict(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "Done, all green."))
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	seq := withHerdrSequence(a, reports, unreads)

	// The reply went to the judge, so the verdict's working→idle is its
	// completion; the synthetic pair would make it two.
	a.Update(agentReply("102.0", "One more note: the flaky test is unrelated."))
	assertHerdrSequence(t, seq, AgentWorking)
	a.Update(verdictFor(t, judged, 1, AgentIdle))
	assertHerdrSequence(t, seq, AgentWorking, AgentIdle)
	if got := lastReport(t, reports); got.status != "2 unread replies" {
		t.Errorf("expected the completion to carry the unread count, got %+v", got)
	}
}

func TestReplyOnIdleThreadJudgedWorkingShowsNoDone(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "Done, all green."))
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(agentReply("102.0", "Actually, let me also fix the flaky test."))
	a.Update(verdictFor(t, judged, 1, AgentWorking))
	// idle→working only: no idle follows a working, so no done.
	assertHerdrSequence(t, seq, AgentWorking)
}

func TestJudgeFailureFallsBackToIdleAndCanBeAskedAgain(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	seq := withHerdrSequence(a, reports, unreads)
	reply := agentReply("101.0", "On it, checking now.")
	a.Update(reply)
	assertHerdrSequence(t, seq)

	// An API error or the timeout: the reply reads idle, as it does with
	// no judge at all.
	a.Update(failureFor(t, judged, 0))
	assertHerdrSequence(t, seq, AgentIdle)

	// The failed ask is forgotten, so a later event for the same message
	// (here a panel reload) asks again.
	openWorkingAgentThread(a, []messages.MessageItem{reply.Message})
	if len(*judged) != 2 || (*judged)[1].key != (*judged)[0].key {
		t.Fatalf("expected a re-ask after the failure, got %+v", *judged)
	}
	a.Update(verdictFor(t, judged, 1, AgentWorking))
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working once the re-ask answered working, got %+v", got)
	}
}

func TestFailedReaskDropsTheHeldVerdict(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(agentReply("101.0", "On it, checking now."))
	a.Update(verdictFor(t, judged, 0, AgentWorking))
	seq := withHerdrSequence(a, reports, unreads)

	a.Update(editedAgentReply("101.0", "Done, all green."))
	// The held verdict answered the old text; with no answer for the new
	// text it can't stand until the next message.
	a.Update(failureFor(t, judged, 1))
	assertHerdrSequence(t, seq, AgentIdle)
}

func TestStaleJudgeFailureLeavesNewerRequestAlone(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	seq := withHerdrSequence(a, reports, unreads)
	a.Update(agentReply("101.0", "On it, checking now."))
	newest := agentReply("102.0", "Found the cause, writing the fix.")
	a.Update(newest)

	a.Update(failureFor(t, judged, 0))
	assertHerdrSequence(t, seq)
	if got := a.agentSidebar.effectiveState(); got != AgentWorking {
		t.Errorf("stale failure ended the newer request's in-flight state: %q", got)
	}
	// The newer request is still the one asked: a reload doesn't refire it.
	openWorkingAgentThread(a, []messages.MessageItem{newest.Message})
	if len(*judged) != 2 {
		t.Errorf("stale failure forgot the newer request: %+v", *judged)
	}
}

func TestOpeningFinishedThreadPublishesNoCompletion(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	seq := withHerdrSequence(a, reports, unreads)
	// A request the panel snapshot fires holds what the thread read before
	// it: opening (or restarting onto) a thread whose agent finished long
	// ago must not read working and then complete.
	openWorkingAgentThread(a, []messages.MessageItem{agentReply("101.0", "Done, all green.").Message})
	if len(*judged) != 1 {
		t.Fatalf("expected the snapshot to ask the judge, got %+v", *judged)
	}
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	assertHerdrSequence(t, seq, AgentIdle)
}

// Every open path paints the root alone before the replies load
// (openThreadPanel, openSelectedThreadCmd). That paint has not read the
// thread, so the user's root on top must not read working, or the loaded
// agent's finished reply publishes working→idle: herdr shows "done" for a
// run that ended long ago.
func TestOpeningFinishedThreadRootFirstPublishesNoCompletion(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	seq := withHerdrSequence(a, reports, unreads)
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> take a look at the CI workflows", UserID: "UHUMAN"}
	a.setThreadPanel(parent, nil, "C1", "100.0")
	a.threadVisible = true
	a.setThreadPanel(parent, []messages.MessageItem{agentReply("101.0", "Done, all green.").Message}, "C1", "100.0")
	if len(*judged) != 1 {
		t.Fatalf("expected the loaded snapshot to ask the judge, got %+v", *judged)
	}
	a.Update(verdictFor(t, judged, 0, AgentIdle))
	assertHerdrSequence(t, seq, AgentIdle)
}

func TestUnjudgedReplyKeepsSyntheticCompletion(t *testing.T) {
	for _, tc := range []struct {
		name  string
		judge bool
		text  string
	}{
		{name: "judge not installed", judge: false, text: "One more note."},
		// A file-only reply has nothing to judge, so no request is fired.
		{name: "empty text", judge: true, text: ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, reports, unreads := newAgentTestApp(t)
			openWorkingAgentThread(a, nil)
			// With no judge installed the plain reply reads idle.
			a.Update(agentReply("101.0", "Done, all green."))
			if tc.judge {
				withWorkingJudge(a)
			}
			seq := withHerdrSequence(a, reports, unreads)

			// No verdict is coming, so the pair is this reply's only
			// completion signal.
			a.Update(agentReply("102.0", tc.text))
			assertHerdrSequence(t, seq, AgentWorking, AgentIdle)
			if len(*unreads) != 1 || (*unreads)[0].status != "2 unread replies" {
				t.Errorf("expected the synthetic completion with the count, got %+v", *unreads)
			}
		})
	}
}

func TestBlockedVerdictReportsBlocked(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "Two options here, A or B. Which do you want?",
	}})
	before := len(*unreads)
	a.Update(verdictFor(t, judged, 0, AgentBlocked))
	// The reply itself is unread: the blocked report carries the count.
	if got := lastReport(t, reports); got.state != AgentBlocked || got.status != "1 unread reply" {
		t.Fatalf("expected a blocked report with the unread count, got %+v", got)
	}

	// Read and unread changes while blocked ride plain blocked reports,
	// never the synthetic working→idle completion that would clear the state.
	a.markAgentThreadRead("", "C1", "100.0")
	if got := lastReport(t, reports); got.state != AgentBlocked || got.status != "" {
		t.Errorf("expected blocked report with no status after read, got %+v", got)
	}
	a.markAgentThreadUnread("", "C1", "100.0", "101.0")
	if got := lastReport(t, reports); got.state != AgentBlocked || got.status != "1 unread reply" {
		t.Errorf("expected blocked report carrying the unread count, got %+v", got)
	}
	if len(*unreads) != before {
		t.Errorf("unread published as a synthetic completion while blocked: %+v", (*unreads)[before:])
	}

	// The user's answer is a human message the agent hasn't acked: working.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "A",
	}})
	if got := lastReport(t, reports); got.state != AgentWorking {
		t.Errorf("expected working after the user's answer, got %+v", got)
	}
}

// usersClaudeCodePost is a post from the user's Claude Code: the user's own
// account, and Slack's Sent using footer at the end of the raw text.
func usersClaudeCodePost(ts, text string) NewMessageMsg {
	return NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: ts, ThreadTS: "100.0", UserID: "UHUMAN", Text: text + " *Sent using* <@U0AJPPX8SE8>",
	}}
}

// A post from the user's Claude Code is an agent session's message: it goes
// to the judge as soon as it arrives, with no bot reaction, and its verdict
// decides. Before, it read working as an unacked user message and was
// never judged, so a session blocked on the user's sign-in showed working.
func TestUsersClaudeCodePostTakesItsVerdict(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(usersClaudeCodePost("101.0", "From Justin's local Claude Code: the live test is blocked on Justin: my gcloud sign-in has expired."))
	if len(*judged) != 1 {
		t.Fatalf("judge calls = %+v, want one for the post", *judged)
	}
	call := (*judged)[0]
	wantEarlier := []string{"user: @Claude take a look at the CI workflows"}
	if call.author != tablabel.AuthorUsersClaudeCode || !strings.HasPrefix(call.key, "101.0|c|") || !slices.Equal(call.earlier, wantEarlier) {
		t.Errorf("judge call = %+v, want the user's Claude Code's side, key 101.0|c|…, earlier %q", call, wantEarlier)
	}
	if got := lastReport(t, reports); got.state != AgentWorking {
		t.Errorf("expected working while the verdict is in flight, got %+v", got)
	}
	a.Update(verdictFor(t, judged, 0, AgentBlocked))
	if got := lastReport(t, reports); got.state != AgentBlocked {
		t.Errorf("expected blocked after a blocked verdict, got %+v", got)
	}
}

// The judge reads the user's Claude Code's posts under their own label, and
// an agent's message after one also gets that session's most recent post,
// including from a panel snapshot whose post is further back than the
// earlier messages reach.
func TestJudgeReadsTheUsersClaudeCodePosts(t *testing.T) {
	post := usersClaudeCodePost("101.0", "From Justin's local Claude Code: after the roll, I'll check the 08:55Z run.")
	merged := agentReply("102.0", "#4300 is merged. The last step is your local Claude Code's check on the 08:55Z run.")
	// The test's user cache has no name for the footer's mention.
	wantPost := "From Justin's local Claude Code: after the roll, I'll check the 08:55Z run. *Sent using* @U0AJPPX8SE8"

	t.Run("from live replies", func(t *testing.T) {
		a, _, _ := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		a.Update(post)
		a.Update(merged)
		if len(*judged) != 2 {
			t.Fatalf("judge calls = %+v, want one per message", *judged)
		}
		call := (*judged)[1]
		want := []string{"user: @Claude take a look at the CI workflows", "user's Claude Code: " + wantPost}
		if call.author != tablabel.AuthorAgent || !slices.Equal(call.earlier, want) || call.usersClaudeCodePost != wantPost {
			t.Errorf("judge call = %+v, want the agent's side, earlier %q and the post %q", call, want, wantPost)
		}
	})

	t.Run("from a panel snapshot", func(t *testing.T) {
		a, _, _ := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		replies := []messages.MessageItem{post.Message}
		for i := range maxJudgeEarlierMsgs {
			replies = append(replies, agentReply(fmt.Sprintf("10%d.5", i+1), "Still checking.").Message)
		}
		openWorkingAgentThread(a, append(replies, merged.Message))
		if len(*judged) != 1 || (*judged)[0].usersClaudeCodePost != wantPost {
			t.Fatalf("judge calls = %+v, want one with the post %q", *judged, wantPost)
		}
	})

	t.Run("not when it has not posted", func(t *testing.T) {
		a, _, _ := newAgentTestApp(t)
		judged := withWorkingJudge(a)
		openWorkingAgentThread(a, nil)
		a.Update(merged)
		if len(*judged) != 1 || (*judged)[0].usersClaudeCodePost != "" {
			t.Fatalf("judge calls = %+v, want one with no post", *judged)
		}
	})
}

// Once the most recent post of the user's Claude Code is deleted, or edited
// so its footer is gone, the judge is no longer told that session posted it,
// whether or not the post was still the thread's newest message: a step an
// agent then hands that session waits for the user to start it.
func TestUsersClaudeCodePostGoneFromTheThreadIsNotItsMostRecentPost(t *testing.T) {
	post := usersClaudeCodePost("101.0", "From Justin's local Claude Code: I'm picking up the live check.")
	unfootered := post
	unfootered.Message.Text, unfootered.Message.IsEdited = "I'm picking up the live check.", true
	for _, tc := range []struct {
		name   string
		gone   any
		newest bool
	}{
		{"deleted while newest", WSMessageDeletedMsg{ChannelID: "C1", TS: "101.0"}, true},
		{"deleted under a later message", WSMessageDeletedMsg{ChannelID: "C1", TS: "101.0"}, false},
		{"edited without the footer while newest", unfootered, true},
		{"edited without the footer under a later message", unfootered, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _, _ := newAgentTestApp(t)
			judged := withWorkingJudge(a)
			openWorkingAgentThread(a, nil)
			a.Update(post)
			if !tc.newest {
				a.Update(agentReply("102.0", "Thanks, the review is up."))
			}
			a.Update(tc.gone)
			a.Update(agentReply("103.0", "The last step is your local Claude Code's run of the steps file below."))
			call := (*judged)[len(*judged)-1]
			if !strings.HasPrefix(call.key, "103.0|") || call.usersClaudeCodePost != "" {
				t.Errorf("judge call = %+v, want one for 103.0 with no post of the user's Claude Code", call)
			}
		})
	}
}

// An edit of the tracked post that keeps the footer is still its post, with
// the edited text.
func TestUsersClaudeCodePostEditedWithTheFooterKeepsItsNewText(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(usersClaudeCodePost("101.0", "From Justin's local Claude Code: I'm picking up the live check."))
	a.Update(agentReply("102.0", "Thanks, the review is up."))
	edit := usersClaudeCodePost("101.0", "From Justin's local Claude Code: I've stopped following this thread.")
	edit.Message.IsEdited = true
	a.Update(edit)
	a.Update(agentReply("103.0", "The last step is your local Claude Code's run of the steps file below."))
	call := (*judged)[len(*judged)-1]
	if want := "From Justin's local Claude Code: I've stopped following this thread. *Sent using* @U0AJPPX8SE8"; call.usersClaudeCodePost != want {
		t.Errorf("judge call = %+v, want the post %q", call, want)
	}
}
