package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

type judgeCall struct {
	teamID, channelID, threadTS, key, message string
	fromAgent                                 bool
}

func withWorkingJudge(a *App) *[]judgeCall {
	calls := &[]judgeCall{}
	a.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, fromAgent bool) {
		*calls = append(*calls, judgeCall{teamID, channelID, threadTS, key, message, fromAgent})
	})
	return calls
}

// withHerdrSequence records everything published to herdr in one ordered
// list, the way herdr sees it: a state report is its state, and the
// synthetic completion is the working→idle pair it puts on the wire
// (herdr.Reporter.ReportUnread). The two recorders newAgentTestApp returns
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
		!call.fromAgent || !strings.Contains(call.message, "go check") {
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

func TestAckReactionAsksJudgeAboutAckedMessage(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call after agent ack, got %+v", *judged)
	}
	call := (*judged)[0]
	if !strings.HasPrefix(call.key, "100.0|h|") || call.fromAgent || !strings.Contains(call.message, "CI workflows") {
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

func TestTodoPostAsksNoJudge(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Picking this up. ✱ Reading. ○ Fixing. _todos as of 19:04 UTC_",
	}})
	if len(*judged) != 0 {
		t.Errorf("judge fired for a todo post: %+v", *judged)
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
