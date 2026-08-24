package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/statusbar"
)

func TestNormalMode_CtrlRRunsReloader(t *testing.T) {
	a := NewApp()
	called := 0
	a.SetReloader(func() { called++ })
	cmd := handleNormalMode(a, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	if called != 1 {
		t.Fatalf("reloader called %d times, want 1", called)
	}
	if cmd == nil {
		t.Fatal("expected the toast-clear cmd")
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Reloading connections") {
		t.Fatalf("expected reload toast, got:\n%s", out)
	}
}

func TestExecuteCommand_ReloadRunsReloader(t *testing.T) {
	a := NewApp()
	called := 0
	a.SetReloader(func() { called++ })
	_ = executeCommand(a, "reload")
	if called != 1 {
		t.Fatalf("reloader called %d times, want 1", called)
	}
}

// TestReload_RefetchesOpenThreadPanel pins the reload → thread-panel
// repair path: a reply swallowed by a websocket gap never reaches an
// open panel any other way (channel backfill can't see thread replies,
// and the panel never re-reads the cache), so ctrl+r must refetch it.
func TestReload_RefetchesOpenThreadPanel(t *testing.T) {
	a := NewApp()
	a.SetReloader(func() {})
	openThreadPanel(a, "C_THREAD", "100.0")
	var fetched []string
	a.setThreadFetcherForTest(func(channelID ids.ChannelID, threadTS ids.ThreadTS) tea.Msg {
		fetched = append(fetched, string(channelID)+"/"+string(threadTS))
		return ThreadRepliesLoadedMsg{ThreadTS: string(threadTS)}
	})

	cmd := a.refetchOpenThreadCmd()
	if cmd == nil {
		t.Fatal("expected a refetch cmd while a thread panel is open")
	}
	if msg, ok := cmd().(ThreadRepliesLoadedMsg); !ok || msg.ThreadTS != "100.0" {
		t.Fatalf("refetch cmd returned %#v, want ThreadRepliesLoadedMsg for 100.0", msg)
	}
	if len(fetched) != 1 || fetched[0] != "C_THREAD/100.0" {
		t.Fatalf("fetched = %v, want [C_THREAD/100.0]", fetched)
	}
}

func TestReload_NoThreadOpenSkipsRefetch(t *testing.T) {
	a := NewApp()
	if cmd := a.refetchOpenThreadCmd(); cmd != nil {
		t.Fatal("no thread open: refetchOpenThreadCmd must be nil")
	}
}

func TestReload_NoReloaderIsNoop(t *testing.T) {
	a := NewApp()
	if cmd := handleNormalMode(a, tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}); cmd != nil {
		t.Fatal("ctrl+r without a wired reloader should be a no-op")
	}
}

// TestReconnectWaitMsgRendersCountdown pins the reconnect-wait
// statusbar flow: the msg switches the segment to a countdown and
// starts exactly one tick chain; a connect stops the chain.
func TestReconnectWaitMsgRendersCountdown(t *testing.T) {
	a := NewApp()
	cmd, handled := reduceIO(a, ConnectionStateMsg{
		State:   int(statusbar.StateReconnecting),
		RetryAt: time.Now().Add(10 * time.Second),
		Attempt: 3,
	})
	if !handled {
		t.Fatal("ConnectionStateMsg not handled")
	}
	if cmd == nil {
		t.Fatal("expected the countdown tick cmd")
	}
	out := a.statusbar.View(120)
	if !strings.Contains(out, "Reconnecting in") || !strings.Contains(out, "(try 3)") {
		t.Fatalf("expected reconnect countdown segment, got:\n%s", out)
	}

	// Second reconnect msg while the chain is live: no second chain.
	cmd, _ = reduceIO(a, ConnectionStateMsg{
		State:   int(statusbar.StateReconnecting),
		RetryAt: time.Now().Add(20 * time.Second),
		Attempt: 4,
	})
	if cmd != nil {
		t.Fatal("second reconnect msg must not start a parallel tick chain")
	}

	// Connected: the next tick stops the chain.
	_, _ = reduceIO(a, ConnectionStateMsg{State: int(statusbar.StateConnected)})
	cmd, _ = reduceIO(a, statusbar.ReconnectTickMsg{})
	if cmd != nil {
		t.Fatal("tick after connect must stop the chain")
	}
	if a.reconnectTickerOn {
		t.Fatal("reconnectTickerOn should be cleared after the chain stops")
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Connected") {
		t.Fatalf("expected connected segment, got:\n%s", out)
	}
}

// TestConnectionStateScopedToActiveWorkspace pins the per-workspace
// segment: an inactive workspace's state is cached, not shown, and a
// workspace switch applies the cached state (Connecting when the
// workspace has never connected).
func TestConnectionStateScopedToActiveWorkspace(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	_, _ = reduceIO(a, ConnectionStateMsg{TeamID: "T1", State: int(statusbar.StateConnected)})

	cmd, _ := reduceIO(a, ConnectionStateMsg{
		TeamID:  "T2",
		State:   int(statusbar.StateReconnecting),
		RetryAt: time.Now().Add(10 * time.Second),
		Attempt: 2,
	})
	if cmd != nil {
		t.Fatal("inactive workspace's reconnect wait must not start a tick chain")
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Connected") || strings.Contains(out, "Reconnecting") {
		t.Fatalf("inactive workspace's state clobbered the active segment:\n%s", out)
	}

	a.activeTeamID = "T2"
	if cmd := a.applyActiveConnState(); cmd == nil {
		t.Fatal("switching to a reconnecting workspace must start the tick chain")
	}
	if out := a.statusbar.View(120); !strings.Contains(out, "Reconnecting in") {
		t.Fatalf("expected T2's cached countdown after switch, got:\n%s", out)
	}

	a.activeTeamID = "T3"
	_ = a.applyActiveConnState()
	if out := a.statusbar.View(120); !strings.Contains(out, "Connecting") {
		t.Fatalf("expected Connecting for a never-connected workspace, got:\n%s", out)
	}
}
