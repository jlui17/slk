package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
)

func withAgentStateRecorder(a *App) *[]AgentStateReport {
	rows := &[]AgentStateReport{}
	a.SetAgentStateRecorder(func(r AgentStateReport) { *rows = append(*rows, r) })
	return rows
}

func lastStateReport(t *testing.T, rows *[]AgentStateReport) AgentStateReport {
	t.Helper()
	if len(*rows) == 0 {
		t.Fatal("no agent state reports recorded")
	}
	return (*rows)[len(*rows)-1]
}

func TestEffectiveStateSources(t *testing.T) {
	agentReply := agentLastMsg{ts: "101.0", authorID: "UBOT"}
	cases := []struct {
		name       string
		sidebar    agentSidebar
		wantState  AgentState
		wantSource AgentStateSource
	}{
		{"live turn wins over everything", agentSidebar{working: true, lastMsg: agentReply}, AgentWorking, SourceAssistantStatus},
		{"no message", agentSidebar{}, AgentIdle, SourceNoMessage},
		{"unacked human", agentSidebar{lastMsg: agentLastMsg{ts: "100.0", human: true}}, AgentWorking, SourceUnackedHuman},
		{"todo post", agentSidebar{lastMsg: agentLastMsg{ts: "101.0", todo: true}}, AgentWorking, SourceTodoPost},
		{"verdict for this key", agentSidebar{lastMsg: agentReply, workingJudge: workingJudgeState{judgedKey: "101.0|a", state: AgentBlocked}}, AgentBlocked, SourceJudge},
		{"verdict for another key", agentSidebar{lastMsg: agentReply, workingJudge: workingJudgeState{judgedKey: "99.0|a", state: AgentBlocked}}, AgentIdle, SourceJudgePending},
		{"acked human, no verdict yet", agentSidebar{lastMsg: agentLastMsg{ts: "100.0", human: true, acked: true}}, AgentIdle, SourceJudgePending},
		{"judge failed for this key", agentSidebar{lastMsg: agentReply, workingJudge: workingJudgeState{failedKey: "101.0|a"}}, AgentIdle, SourceJudgeError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, source := tc.sidebar.effective()
			if state != tc.wantState || source != tc.wantSource {
				t.Errorf("effective() = %v, %v; want %v, %v", state, source, tc.wantState, tc.wantSource)
			}
			if got := tc.sidebar.effectiveState(); got != tc.wantState {
				t.Errorf("effectiveState() = %v, want %v", got, tc.wantState)
			}
		})
	}
}

// Every report to herdr logs one row, so the two counts stay in step.
func TestStateReportRowPerHerdrReport(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)

	// Tracking starts with an idle report before the snapshot sees the
	// unanswered root.
	want := []AgentStateReport{
		{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", State: AgentIdle, Source: SourceNoMessage},
		{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", MessageTS: "100.0", State: AgentWorking, Source: SourceUnackedHuman},
	}
	if len(*rows) != len(want) || (*rows)[0] != want[0] || (*rows)[1] != want[1] {
		t.Fatalf("rows = %+v, want %+v", *rows, want)
	}

	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})
	if got := lastStateReport(t, rows); got.State != AgentWorking || got.Source != SourceAssistantStatus || got.MessageTS != "100.0" {
		t.Errorf("row during a live turn = %+v", got)
	}

	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "✱ Reading the workflows. ○ Report back.",
	}})
	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})
	if got := lastStateReport(t, rows); got.State != AgentWorking || got.Source != SourceTodoPost || got.MessageTS != "101.0" || !got.FromAgent {
		t.Errorf("row for a todo post = %+v", got)
	}
	if len(*rows) != len(*reports) {
		t.Errorf("%d rows for %d herdr reports", len(*rows), len(*reports))
	}
}

func TestJudgeVerdictRowCarriesJudgeDetails(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "A or B?",
	}})
	if got := lastStateReport(t, rows); got.State != AgentIdle || got.Source != SourceJudgePending {
		t.Errorf("row while the verdict is in flight = %+v", got)
	}

	a.Update(AgentWorkingVerdictMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentBlocked,
		Reply: "u", Model: "claude-haiku-4-5", PromptHash: "p1", TextHash: "t1",
	})
	want := AgentStateReport{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", MessageTS: "101.0", FromAgent: true,
		State: AgentBlocked, Source: SourceJudge,
		JudgeReply: "u", JudgeModel: "claude-haiku-4-5", JudgePromptHash: "p1", MessageTextHash: "t1",
	}
	if got := lastStateReport(t, rows); got != want {
		t.Errorf("row = %+v, want %+v", got, want)
	}

	// A republish of the standing verdict keeps the details; a newer
	// message's row does not inherit them.
	a.Update(HerdrConnectedMsg{})
	if got := lastStateReport(t, rows); got != want {
		t.Errorf("republished row = %+v, want %+v", got, want)
	}
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "B",
	}})
	if got := lastStateReport(t, rows); got.Source != SourceUnackedHuman || got.JudgeReply != "" || got.JudgeModel != "" {
		t.Errorf("row for the next message = %+v", got)
	}
}

// An idle verdict changes nothing herdr shows, so nothing is reported, but
// the log still needs the row to tell whether "idle" was right.
func TestIdleVerdictIsLoggedWithoutAHerdrReport(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "Done, PR is up.",
	}})
	reported := len(*reports)

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", State: AgentIdle, Reply: "d"})
	if got := lastStateReport(t, rows); got.State != AgentIdle || got.Source != SourceJudge || got.JudgeReply != "d" {
		t.Errorf("row = %+v", got)
	}
	if len(*reports) != reported {
		t.Errorf("an idle verdict on an idle row reported to herdr: %+v", (*reports)[reported:])
	}
}

func TestJudgeErrorIsLogged(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	judged := withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	reported := len(*reports)

	a.Update(AgentWorkingVerdictMsg{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a",
		Err: "context deadline exceeded", Model: "claude-haiku-4-5", PromptHash: "p1", TextHash: "t1",
	})
	want := AgentStateReport{
		TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", MessageTS: "101.0", FromAgent: true,
		State: AgentIdle, Source: SourceJudgeError,
		JudgeModel: "claude-haiku-4-5", JudgePromptHash: "p1", MessageTextHash: "t1",
		Error: "context deadline exceeded",
	}
	if got := lastStateReport(t, rows); got != want {
		t.Errorf("row = %+v, want %+v", got, want)
	}
	if len(*reports) != reported {
		t.Errorf("a judge error reported to herdr: %+v", (*reports)[reported:])
	}

	// No retry: the same state is not asked about again.
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	if len(*judged) != 1 {
		t.Errorf("judge calls = %d, want 1", len(*judged))
	}
}

func TestStaleJudgeErrorIsNotLogged(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "101.0", ThreadTS: "100.0", UserID: "UBOT", Text: "let me check",
	}})
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "102.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "also look at CI",
	}})
	recorded := len(*rows)

	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: "101.0|a", Err: "timeout"})
	if len(*rows) != recorded {
		t.Errorf("a stale judge error was logged: %+v", (*rows)[recorded:])
	}
}
