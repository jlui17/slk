package ui

import (
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
		call.key != "101.0|a" || !call.fromAgent || !strings.Contains(call.message, "go check") {
		t.Errorf("judge call = %+v", call)
	}
	// Until the verdict lands, the plain reply reads idle as before.
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle while verdict in flight, got %+v", got)
	}

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentWorking})
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
	if call.key != "100.0|h" || call.fromAgent || !strings.Contains(call.message, "CI workflows") {
		t.Errorf("judge call = %+v", call)
	}
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle while verdict in flight, got %+v", got)
	}
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "100.0|h", State: AgentWorking})
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working after a working verdict on acked ask, got %+v", got)
	}
}

func TestStaleVerdictIsDropped(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	// The thread moves on before the verdict lands.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "also this",
	}})
	before := len(*reports)
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentIdle})
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
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "On it, checking now.",
	}})
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentWorking})
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
		t.Errorf("expected a re-ask with the edited text, got %+v", *judged)
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
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentBlocked})
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
