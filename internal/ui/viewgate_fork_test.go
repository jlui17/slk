package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
)

func TestView_UnviewedPaneReturnsLastViewUnchanged(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateConnected)})
	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v1 := a.View()

	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: "3.0", UserName: "carol", UserID: "U3", Text: "changed content", Timestamp: "1:02 PM"},
	})
	v2 := a.View()
	if v2.Content != v1.Content {
		t.Fatalf("unviewed View() recomputed the screen; want the memoized view back")
	}

	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v3 := a.View()
	if v3.Content == v1.Content {
		t.Fatalf("View() after becoming viewed returned the stale screen; want a fresh render")
	}
}

func TestView_UnviewedPaneRerendersOnResize(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateConnected)})
	_, _ = a.Update(HerdrTabViewMsg{Viewed: true})
	v1 := a.View()

	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.width = 100
	v2 := a.View()
	if v2.Content == v1.Content {
		t.Fatalf("unviewed View() after a resize returned the old-size screen; want a re-render at the new size")
	}
}

func TestView_UnviewedPaneAnimatesBootstrapOverlay(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.SetLoadingWorkspaces([]string{"acme"})
	v1 := a.View()

	_, _ = a.Update(SpinnerTickMsg{})
	v2 := a.View()
	if v2.Content == v1.Content {
		t.Fatalf("unviewed View() froze the bootstrap overlay; want a fresh render with the advanced spinner frame")
	}
}

func TestView_UnviewedPaneShowsBootstrapDismissal(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	a.SetLoadingWorkspaces([]string{"acme"})
	overlay := a.View()

	a.bootstrap.MarkReady("acme")
	v := a.View()
	if v.Content == overlay.Content {
		t.Fatalf("unviewed View() after overlay dismissal served the stale loading screen; want the real UI")
	}
}

func TestView_OutsideHerdrViewIsNeverGated(t *testing.T) {
	a := newTestAppWithMessages(t)
	v1 := a.View()
	a.messagepane.SetMessages([]messages.MessageItem{
		{TS: "3.0", UserName: "carol", UserID: "U3", Text: "changed content", Timestamp: "1:02 PM"},
	})
	v2 := a.View()
	if v2.Content == v1.Content {
		t.Fatalf("View() outside herdr returned a stale screen; the gate must only fire when a pane is known unviewed")
	}
}

// TestView_UnviewedPaneShowsConnectAfterCachedBoot is a launch into an
// unviewed tab on the cache-first boot: the cached paint dismisses the
// overlay while the status bar still reads connecting and syncing, so
// the frame the pane is left on must be one drawn after both settle.
func TestView_UnviewedPaneShowsConnectAfterCachedBoot(t *testing.T) {
	cached := []messages.MessageItem{{TS: "1700000001.000000", Text: "from cache"}}
	a := cachedBootHarness(t, cached, nil)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})

	_, cmd := a.Update(cachedWorkspaceFixture())
	sel, _ := findChannelSelected(cmd())
	_, _ = a.Update(sel)
	_ = a.View()

	_, cmd = a.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		TeamName:      "Acme",
		InitialActive: true,
		Channels:      cachedWorkspaceFixture().Channels,
		LastChannelID: "C2",
	})
	sel, _ = findChannelSelected(cmd())
	_, cmd = a.Update(sel)
	drainCmds(cmd)
	_ = a.View()

	_, _ = a.Update(ConnectionStateMsg{TeamID: "T1", State: int(statusbar.StateConnected)})
	_ = a.View()
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: "C2", Messages: cached})

	v := a.View().Content
	if !strings.Contains(v, "Connected") || strings.Contains(v, "Connecting") || strings.Contains(v, "syncing") {
		t.Fatalf("unviewed pane left on a boot-era frame; want the connected, synced status bar")
	}
}

// TestView_UnviewedPaneShowsReconnect is a launch into an unviewed tab
// whose first dial fails: a redial goes Disconnected, Reconnecting,
// Connected with no Connecting between, and the pane must not be left
// on the Disconnected frame once the hello lands.
func TestView_UnviewedPaneShowsReconnect(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	_ = a.View()

	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateDisconnected)})
	_ = a.View()
	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateReconnecting), RetryAt: time.Now().Add(time.Second), Attempt: 1})
	_ = a.View()
	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateConnected)})

	v := a.View().Content
	if !strings.Contains(v, "● Connected") {
		t.Fatalf("unviewed pane left on a pre-reconnect frame; want the connected status bar")
	}
}

// TestView_UnviewedPaneFreezesReconnectCountdown: the countdown ticks
// every second through a long outage without changing the connection
// state, so an unviewed pane keeps the frame it has.
func TestView_UnviewedPaneFreezesReconnectCountdown(t *testing.T) {
	a := newTestAppWithMessages(t)
	_, _ = a.Update(HerdrTabViewMsg{Viewed: false})
	_, _ = a.Update(ConnectionStateMsg{State: int(statusbar.StateReconnecting), RetryAt: time.Now().Add(30 * time.Second), Attempt: 3})
	v1 := a.View()

	a.statusbar.SetReconnectWait(time.Now().Add(10*time.Second), 3)
	v2 := a.View()
	if v2.Content != v1.Content {
		t.Fatalf("unviewed View() redrew for a countdown tick; want the memoized view back")
	}
}
