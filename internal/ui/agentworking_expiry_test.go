package ui

import (
	"testing"
	"time"

	"github.com/gammons/slk/internal/ui/messages"
)

// withAgentClock puts the tracked thread on a clock the test moves. The
// test threads' messages are stamped around ts 100.
func withAgentClock(a *App, startSec int64) *time.Time {
	now := time.Unix(startSec, 0)
	a.agentSidebar.nowFn = func() time.Time { return now }
	return &now
}

const expirySecs = int64(agentWorkingExpiry / time.Second)

func TestWorkingExpiresAfterSilence(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	rows := withAgentStateRecorder(a)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)
	if got := lastReport(t, reports); !got.working {
		t.Fatalf("expected working on the unanswered root, got %+v", got)
	}

	*now = time.Unix(100+expirySecs-1, 0)
	reported := len(*reports)
	a.Update(agentWorkingExpiryTickMsg{})
	if len(*reports) != reported {
		t.Fatalf("a tick inside the period reported: %+v", (*reports)[reported:])
	}

	*now = time.Unix(100+expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); got.state != AgentIdle {
		t.Errorf("expected idle once the period ran out, got %+v", got)
	}
	if got := lastStateReport(t, rows); got.State != AgentIdle || got.Source != SourceWorkingExpired || got.MessageTS != "100.0" {
		t.Errorf("row = %+v", got)
	}

	// The expiry is one edge: later ticks have nothing to say.
	reported = len(*reports)
	a.Update(agentWorkingExpiryTickMsg{})
	if len(*reports) != reported {
		t.Errorf("a tick after the expiry reported again: %+v", (*reports)[reported:])
	}

	// A new message is a new period.
	*now = time.Unix(5000, 0)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "5000.0", ThreadTS: "100.0", UserID: "UHUMAN", Text: "are you still on this?",
	}})
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expected working again after a new message, got %+v", got)
	}
}

func TestNewMessageInsideThePeriodRestartsIt(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)

	*now = time.Unix(500, 0)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "500.0", ThreadTS: "100.0", UserID: "UBOT", Text: "✱ Reading the workflows. ○ Report back.",
	}})
	*now = time.Unix(100+expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); !got.working {
		t.Fatalf("the root's period expired a thread that had moved on: %+v", got)
	}
	*now = time.Unix(500+expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle once the todo post's own period ran out, got %+v", got)
	}
}

// A message already older than the period when slk first sees it never
// reads working: nothing latches until a tick comes round.
func TestOldMessageReadsExpiredAtOnce(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withAgentClock(a, 100+3*24*3600)
	openWorkingAgentThread(a, nil)
	for _, r := range *reports {
		if r.working {
			t.Fatalf("an old unanswered root was reported working: %+v", *reports)
		}
	}
	if state, source := a.agentSidebar.effective(); state != AgentIdle || source != SourceWorkingExpired {
		t.Errorf("effective() = %v, %v; want idle, working_expired", state, source)
	}
}

// The period counts from the end of the last live turn when that is later
// than the newest message.
func TestLiveTurnEndRestartsThePeriod(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)
	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: "is thinking…"})

	// A live turn is never expired, however long it runs.
	*now = time.Unix(100+2*expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); !got.working || got.status != "is thinking…" {
		t.Fatalf("a live turn expired: %+v", got)
	}

	a.Update(AssistantStatusMsg{ChannelID: "C1", ThreadTS: "100.0", BotUserID: "UBOT", Status: ""})
	if got := lastReport(t, reports); !got.working {
		t.Fatalf("expected the unanswered root to read working after the turn, got %+v", got)
	}
	*now = now.Add(agentWorkingExpiry - time.Second)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); !got.working {
		t.Errorf("expired before the period since the turn's end ran out: %+v", got)
	}
	*now = now.Add(time.Second)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle a full period after the turn's end, got %+v", got)
	}
}

// "On it, will report" followed by a dead agent: the judge's working
// verdict expires like any other, and the row keeps what the judge said.
func TestJudgedWorkingExpires(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	rows := withAgentStateRecorder(a)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "150.0", ThreadTS: "100.0", UserID: "UBOT", Text: "On it, will report.",
	}})
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentWorking, Reply: "w says it will report"})
	if got := lastReport(t, reports); !got.working {
		t.Fatalf("expected working after the verdict, got %+v", got)
	}

	*now = time.Unix(150+expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); got.working {
		t.Errorf("expected idle once the period ran out, got %+v", got)
	}
	if got := lastStateReport(t, rows); got.Source != SourceWorkingExpired || got.JudgeReply != "w says it will report" {
		t.Errorf("row = %+v", got)
	}
}

// Blocked waits on the user, so the agent's silence says nothing about it.
func TestBlockedDoesNotExpire(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	withWorkingJudge(a)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "150.0", ThreadTS: "100.0", UserID: "UBOT", Text: "A or B?",
	}})
	a.Update(AgentWorkingVerdictMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", Key: judgeKey(a), State: AgentBlocked})

	*now = time.Unix(150+6*expirySecs, 0)
	a.Update(agentWorkingExpiryTickMsg{})
	if got := lastReport(t, reports); got.state != AgentBlocked {
		t.Errorf("expected blocked to stand, got %+v", got)
	}
}

// An event that lands after the period ran out but before a tick noticed
// must not swallow the edge: derived() already reads idle, so the handler
// alone would compare idle with idle and leave herdr on working.
func TestEventAfterUnreportedExpiryReportsIt(t *testing.T) {
	a, reports, _ := newAgentTestApp(t)
	rows := withAgentStateRecorder(a)
	now := withAgentClock(a, 160)
	openWorkingAgentThread(a, nil)

	*now = time.Unix(100+expirySecs+30, 0)
	a.Update(NewMessageMsg{ChannelID: "C1", Message: messages.MessageItem{
		TS: "730.0", ThreadTS: "100.0", UserID: "UBOT", Text: "Done, PR is up.",
	}})
	if got := lastReport(t, reports); got.working {
		t.Errorf("herdr was left on working: %+v", got)
	}
	if got := lastStateReport(t, rows); got.Source != SourceWorkingExpired || got.MessageTS != "100.0" {
		t.Errorf("expected the expiry to be logged against the root, got %+v", got)
	}
}

// One chain, however often herdr reconnects.
func TestExpiryTickChainIsClaimedOnce(t *testing.T) {
	a, _, _ := newAgentTestApp(t)
	if _, cmd := a.Update(HerdrConnectedMsg{}); cmd == nil {
		t.Fatal("the first herdr connection did not start the expiry tick")
	}
	if _, cmd := a.Update(HerdrConnectedMsg{}); cmd != nil {
		t.Error("a reconnect started a second tick chain")
	}
	if _, cmd := a.Update(agentWorkingExpiryTickMsg{}); cmd == nil {
		t.Error("a tick did not schedule the next one")
	}
}
