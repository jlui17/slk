package ui

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
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

// judgeKey is the key of the tracked thread's newest message as it stands.
func judgeKey(a *App) string {
	return workingJudgeKey(a.agentSidebar.lastMsg)
}

func TestPlainAgentReplyAsksJudge(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "true, let me go check the workflows",
	}})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call, got %+v", *judged)
	}
	call := (*judged)[0]
	if call.teamID != "T1" || call.channelID != "C1" || call.threadTS != "100.0" ||
		call.key != judgeKey(a) || !call.fromAgent || !strings.Contains(call.message, "go check") {
		t.Errorf("judge call = %+v", call)
	}
	// Until the verdict lands, the plain reply reads idle as before.
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle while verdict in flight, got %+v", got)
	}

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentWorking})
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working after a working verdict, got %+v", got)
	}
}

// The user's own "thanks" must not latch working until the agent happens
// to react: it reads working only while the verdict is out.
func TestUnackedHumanMessageAsksJudge(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, []messages.MessageItem{
		{TS: "101.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "thanks!"},
	})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call for the unacked message, got %+v", *judged)
	}
	if call := (*judged)[0]; call.key != judgeKey(a) || call.fromAgent || call.message != "thanks!" {
		t.Errorf("judge call = %+v", call)
	}
	if got := lastStateReport(t, rows); got.State != AgentWorking || got.Source != SourceUnackedHuman {
		t.Errorf("row while the verdict is in flight = %+v", got)
	}

	// A failed request decides nothing: still working, as without a judge.
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), Err: "timeout"})
	if got := lastStateReport(t, rows); got.State != AgentWorking || got.Source != SourceUnackedHuman || got.Error != "timeout" {
		t.Errorf("row after a judge error = %+v", got)
	}

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentIdle, Reply: "n only thanks"})
	if got := lastStateReport(t, rows); got.State != AgentIdle || got.Source != SourceJudge || got.JudgeReply != "n only thanks" {
		t.Errorf("row after an idle verdict = %+v", got)
	}
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle after the verdict, got %+v", got)
	}
}

// The ack doesn't change the question, so a message judged before the
// agent reacted keeps its verdict and costs no second request.
func TestAckAfterVerdictDoesNotReask(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentWorking})
	if got := lastStateReport(t, rows); got.State != AgentWorking || got.Source != SourceJudge {
		t.Fatalf("row after a working verdict = %+v", got)
	}

	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if len(*judged) != 1 {
		t.Errorf("the ack re-asked the judge: %+v", *judged)
	}
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected the working verdict to stand after the ack, got %+v", got)
	}
}

func TestAckWhileVerdictInFlightReadsIdleUntilItLands(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(ReactionAddedMsg{ChannelID: "C1", MessageTS: "100.0", UserID: "UBOT", Emoji: "rocket"})
	if len(*judged) != 1 {
		t.Fatalf("expected the one judge call from the open, got %+v", *judged)
	}
	call := (*judged)[0]
	if call.key != judgeKey(a) || call.fromAgent || !strings.Contains(call.message, "CI workflows") {
		t.Errorf("judge call = %+v", call)
	}
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle while verdict in flight, got %+v", got)
	}
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentWorking})
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working after a working verdict on acked ask, got %+v", got)
	}
}

// Numbered options and a closing question on their own lines are what the
// judge reads a hand-off from. An agent post's Text is Slack's fallback
// with the newlines already flattened away; its rich_text blocks keep them,
// on both sides of a table.
func TestJudgeTextKeepsLines(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Two ways: 1. fail the run 2. log a WARN Which one?",
		Blocks: []blockkit.Block{
			blockkittest.Paragraph("Two   ways:\n1. fail the run\n2. log a WARN"),
			blockkit.TableBlock{Rows: [][]string{{"option", "cost"}}},
			blockkittest.Paragraph("\n\n\nWhich one?"),
		},
	}})
	if len(*judged) != 1 {
		t.Fatalf("expected one judge call, got %+v", *judged)
	}
	if got, want := (*judged)[0].message, "Two ways:\n1. fail the run\n2. log a WARN\n\nWhich one?"; got != want {
		t.Errorf("agent judge text = %q, want %q", got, want)
	}

	// Without blocks the Text is judged, mentions resolved, lines kept.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "<@UBOT> two things:\n- rebase\n- rerun the check",
	}})
	if got, want := (*judged)[len(*judged)-1].message, "@Claude two things:\n- rebase\n- rerun the check"; got != want {
		t.Errorf("human judge text = %q, want %q", got, want)
	}
}

func TestStaleVerdictIsDropped(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	stale := judgeKey(a)
	// The thread moves on before the verdict lands.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "also this",
	}})
	before := len(*reports)
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: stale, State: AgentIdle})
	if len(*reports) != before {
		t.Errorf("stale verdict published a report: %+v", (*reports)[before:])
	}
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working from the unacked follow-up, got %+v", got)
	}
}

func TestPendingTodoPostAsksNoJudge(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Picking this up. ✱ Reading. ○ Fixing. _todos as of 19:04 UTC_",
	}})
	if len(*judged) != 0 {
		t.Errorf("judge fired for a todo post: %+v", *judged)
	}
}

// A list with nothing left open says nothing about what the agent does
// next, so it is judged like any other reply instead of latching working.
func TestAllDoneTodoPostAsksJudge(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT",
		Text: "Reviewing the workflows. ✓ Read them. ✓ Findings posted below. _todos as of 19:20 UTC_",
	}})
	if len(*judged) != 1 || !(*judged)[0].fromAgent || !strings.Contains((*judged)[0].message, "Findings posted") {
		t.Fatalf("expected one agent-side judge call for an all-done list, got %+v", *judged)
	}
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle while the verdict is in flight, got %+v", got)
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
	a, reports, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "On it, checking now.",
	}})
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentWorking})
	if got := lastReport(t, reports); !got.working {
		t.Fatalf("expected working after a working verdict, got %+v", got)
	}
	// The edit changes what was judged: the old verdict is dropped (idle
	// again) and the new text is re-asked.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", IsEdited: true, Text: "Done, all green.",
	}})
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle after edit dropped the verdict, got %+v", got)
	}
	if len(*judged) != 2 || !strings.Contains((*judged)[1].message, "all green") {
		t.Fatalf("expected a re-ask with the edited text, got %+v", *judged)
	}
}

// The request for the old text can still be in flight when the edit is
// re-asked, and the two replies can land in either order.
func TestVerdictForTextBeforeEditIsDropped(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	openWorkingAgentThread(a, nil)
	judged := withWorkingJudge(a)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "On it, checking now.",
	}})
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", IsEdited: true, Text: "Done, all green.",
	}})
	if len(*judged) != 2 || (*judged)[0].key == (*judged)[1].key {
		t.Fatalf("expected the edit to be asked under its own key, got %+v", *judged)
	}
	oldText, newText := (*judged)[0].key, (*judged)[1].key

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: newText, State: AgentIdle})
	before := len(*reports)
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: oldText, State: AgentWorking})
	if len(*reports) != before {
		t.Errorf("the old text's verdict published a report: %+v", (*reports)[before:])
	}
	if state, source := a.agentSidebar.effective(); state != AgentIdle || source != SourceJudge {
		t.Errorf("effective() = %v, %v; want the edited text's idle verdict", state, source)
	}
}

func TestBlockedVerdictReportsBlocked(t *testing.T) {
	a, reports, unreads := newAgentTestApp(t)
	withWorkingJudge(a)
	openWorkingAgentThread(a, nil)

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "Two options here, A or B. Which do you want?",
	}})
	before := len(*unreads)
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentBlocked})
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
