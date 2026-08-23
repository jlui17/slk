package statusbar

import (
	"strings"
	"testing"
	"time"
)

func TestReconnectSegment(t *testing.T) {
	m := New()

	m.SetReconnectWait(time.Now().Add(10*time.Second), 1)
	out := m.View(120)
	if !strings.Contains(out, "Reconnecting in") {
		t.Fatalf("expected countdown segment, got:\n%s", out)
	}
	if strings.Contains(out, "(try") {
		t.Fatalf("first attempt must not render a try counter, got:\n%s", out)
	}

	m.SetReconnectWait(time.Now().Add(10*time.Second), 4)
	if out := m.View(120); !strings.Contains(out, "(try 4)") {
		t.Fatalf("expected try counter from the second attempt on, got:\n%s", out)
	}

	// Past deadline: redial in flight, countdown dropped.
	m.SetReconnectWait(time.Now().Add(-time.Second), 2)
	out = m.View(120)
	if strings.Contains(out, "Reconnecting in") {
		t.Fatalf("past-deadline segment must not render a countdown, got:\n%s", out)
	}
	if !strings.Contains(out, "Reconnecting") {
		t.Fatalf("expected plain reconnecting label, got:\n%s", out)
	}
}

func TestTickReconnect(t *testing.T) {
	m := New()
	if m.TickReconnect() {
		t.Fatal("TickReconnect must report false outside StateReconnecting")
	}
	m.SetReconnectWait(time.Now().Add(5*time.Second), 1)
	v := m.Version()
	if !m.TickReconnect() {
		t.Fatal("TickReconnect must report true while reconnecting")
	}
	if m.Version() == v {
		t.Fatal("TickReconnect must dirty the bar so the countdown re-renders")
	}
	m.SetConnectionState(StateConnected)
	if m.TickReconnect() {
		t.Fatal("TickReconnect must report false after a connect")
	}
}
