package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/gammons/slk/internal/ui/presencemenu"
	"github.com/gammons/slk/internal/ui/statusbar"
)

func TestTypingExpireCrossesTTL(t *testing.T) {
	tr := newTypingTracker()
	now := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	tr.SetNowFunc(func() time.Time { return now })

	tr.Add("C1", "U1")
	tr.Add("C1", "U2")
	tr.MarkTickerOn()
	if got := tr.Users("C1"); len(got) != 2 {
		t.Fatalf("expected 2 typers before TTL, got %v", got)
	}

	now = now.Add(typingExpiry + time.Second)
	if tr.Expire() {
		t.Error("Expire: want false once the TTL has passed for every typer")
	}
	if got := tr.Users("C1"); len(got) != 0 {
		t.Errorf("expected no typers after TTL, got %v", got)
	}
	if tr.TickerOn() {
		t.Error("tickerOn must clear when the last typer expires")
	}
}

func TestSelfSendInFlightExpiresAfterWindow(t *testing.T) {
	d := newSelfSendDedup()
	now := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	d.SetNowFunc(func() time.Time { return now })

	d.MarkInFlight("C1")
	if !d.InFlight("C1") {
		t.Fatal("expected in-flight immediately after MarkInFlight")
	}

	now = now.Add(selfSendWindow)
	if d.InFlight("C1") {
		t.Error("expected in-flight window closed after selfSendWindow")
	}
}

func TestSelfSendRecordSentPrunesStaleEntries(t *testing.T) {
	d := newSelfSendDedup()
	now := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	d.SetNowFunc(func() time.Time { return now })

	first := "1700000000.000000"
	d.RecordSent(first)
	for i := 1; i < 65; i++ {
		d.RecordSent(fmt.Sprintf("1700000000.%06d", i))
	}
	if !d.IsSelfSent(first) {
		t.Fatal("fresh entry must survive the opportunistic cleanup")
	}

	now = now.Add(6 * time.Minute)
	late := "1700000400.000000"
	d.RecordSent(late)
	if d.IsSelfSent(first) {
		t.Error("entry older than 5 minutes must be pruned once the map exceeds 64")
	}
	if !d.IsSelfSent(late) {
		t.Error("the entry just recorded must survive the cleanup")
	}
}

func TestDNDTickExpiryUsesInjectedClock(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	now := time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	a.presence.SetNowFunc(func() time.Time { return now })
	a.presence.Set("T1", "active", true, now.Add(30*time.Minute))
	a.presence.dndTickerOn = true

	cmd, handled := a.presence.Handle(a, statusbar.DNDTickMsg{})
	if !handled {
		t.Fatal("DNDTickMsg must be handled by the presence reducer")
	}
	if cmd == nil {
		t.Fatal("expected a reschedule while DND is still active")
	}

	now = now.Add(31 * time.Minute)
	cmd, _ = a.presence.Handle(a, statusbar.DNDTickMsg{})
	if cmd != nil {
		t.Error("expected no reschedule once DND expired")
	}
	if st := a.presence.byTeam["T1"]; st.DNDEnabled {
		t.Error("cached DND flag must clear on local expiry")
	}
	if a.presence.dndTickerOn {
		t.Error("ticker claim must clear on local expiry")
	}
}

func TestNowFormattedFallbackUsesInjectedClock(t *testing.T) {
	a := NewApp()
	a.SetNowFunc(func() time.Time {
		return time.Date(2026, 8, 19, 14, 7, 0, 0, time.UTC)
	})
	if got := a.nowFormatted(); got != "2:07 PM" {
		t.Errorf("nowFormatted: want %q, got %q", "2:07 PM", got)
	}
}

func TestSnoozeApplyStampsInjectedClock(t *testing.T) {
	p := newPresenceController()
	now := time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC)
	p.SetNowFunc(func() time.Time { return now })

	st := p.Apply("T1", presencemenu.ActionSnooze, 20)
	if want := now.Add(20 * time.Minute); !st.DNDEndTS.Equal(want) {
		t.Fatalf("DNDEndTS = %v, want %v (Apply must stamp the injected clock, not wall time)", st.DNDEndTS, want)
	}
}
