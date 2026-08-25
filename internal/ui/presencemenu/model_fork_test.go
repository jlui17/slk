package presencemenu

import (
	"testing"
	"time"
)

func snoozeUntilTomorrowMinutes(t *testing.T, m Model) int {
	t.Helper()
	for _, it := range m.items {
		if it.label == "Snooze until tomorrow morning" {
			return it.minutes
		}
	}
	t.Fatal("no 'Snooze until tomorrow morning' row")
	return 0
}

func TestSnoozeUntilTomorrowMorningUsesInjectedClock(t *testing.T) {
	var m Model
	// Wednesday 14:00 -> Thursday 09:00.
	m.SetNowFunc(func() time.Time {
		return time.Date(2026, 8, 19, 14, 0, 0, 0, time.UTC)
	})
	m.OpenWith("WS", "active", false, time.Time{})
	if got, want := snoozeUntilTomorrowMinutes(t, m), 19*60; got != want {
		t.Errorf("minutes until tomorrow morning: want %d, got %d", want, got)
	}
}

func TestSnoozeUntilTomorrowMorningSkipsWeekend(t *testing.T) {
	var m Model
	// Friday 18:00 -> Monday 09:00.
	m.SetNowFunc(func() time.Time {
		return time.Date(2026, 8, 21, 18, 0, 0, 0, time.UTC)
	})
	m.OpenWith("WS", "active", false, time.Time{})
	if got, want := snoozeUntilTomorrowMinutes(t, m), 63*60; got != want {
		t.Errorf("minutes until Monday morning: want %d, got %d", want, got)
	}
}

func TestDNDActiveJudgedByInjectedClock(t *testing.T) {
	var m Model
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m.SetNowFunc(func() time.Time { return base })
	// dndEnd is in the past relative to the real wall clock but in the
	// future relative to the injected one; only the injected clock may
	// decide.
	m.OpenWith("WS", "active", true, base.Add(time.Hour))
	if !m.hasEndDNDItem() {
		t.Error("dndEnd after the injected now must be treated as DND active")
	}
}
