package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// cachedBootHarness builds an App the way a real boot starts: loading
// overlay armed, and a ChannelService whose ReadCache serves cachedMsgs
// with a stale synced_at (the shape a prior session's sqlite cache
// presents). fetches records network fetch dispatches.
func cachedBootHarness(t *testing.T, cachedMsgs []messages.MessageItem, fetches *[]string) *App {
	t.Helper()
	return newHarnessApp(t, withApp(func(a *App) {
		a.SetLoadingWorkspaces([]string{"Acme"})
		a.SetChannelService(NewChannelService(ChannelServiceFuncs{
			ReadCache: func(id ids.ChannelID) []messages.MessageItem { return cachedMsgs },
			SyncedAt:  func(id ids.ChannelID) int64 { return time.Now().Add(-time.Hour).Unix() },
			Fetch: func(id ids.ChannelID, name string) tea.Msg {
				if fetches != nil {
					*fetches = append(*fetches, string(id))
				}
				return nil
			},
		}))
	}))
}

func cachedWorkspaceFixture() WorkspaceCachedMsg {
	return WorkspaceCachedMsg{
		TeamID:   "T1",
		TeamName: "Acme",
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "C2", Name: "random", Type: "channel"},
		},
		LastChannelID: "C2",
	}
}

// TestWorkspaceCachedPaintsProvisionally is the headline: before any
// WorkspaceReadyMsg, a seeded cache paints the sidebar, dismisses the
// connecting overlay, selects the restored channel so its cached
// messages render, and raises the "cached · syncing" pill.
func TestWorkspaceCachedPaintsProvisionally(t *testing.T) {
	cached := []messages.MessageItem{{TS: "1700000001.000000", Text: "from cache"}}
	a := cachedBootHarness(t, cached, nil)

	_, cmd := a.Update(cachedWorkspaceFixture())

	if a.bootstrap.IsLoading() {
		t.Error("connecting overlay still up over cache-painted content")
	}
	if got := len(a.sidebar.Items()); got != 2 {
		t.Errorf("sidebar has %d items, want 2 from cache", got)
	}
	if a.activeTeamID != "T1" {
		t.Errorf("activeTeamID = %q, want provisional T1", a.activeTeamID)
	}
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != "C2" {
		t.Fatalf("want restored channel C2 selected, got ok=%v id=%q", ok, sel.ID)
	}
	_, _ = a.Update(sel)
	if got := a.messagepane.Messages(); len(got) != 1 || got[0].Text != "from cache" {
		t.Errorf("messagepane = %v, want the cached message rendered", got)
	}
	if !strings.Contains(a.statusbar.View(120), "cached · syncing") {
		t.Error("statusbar missing the cached · syncing pill during provisional boot")
	}
}

func TestWorkspaceCachedNoRestoredChannelSelectsNothing(t *testing.T) {
	a := cachedBootHarness(t, nil, nil)
	m := cachedWorkspaceFixture()
	m.LastChannelID = ""
	_, cmd := a.Update(m)
	if msgs := drainCmds(cmd); len(msgs) != 0 {
		t.Errorf("no recorded restore must select nothing, got %v", msgs)
	}
	if a.activeChannelID != "" {
		t.Errorf("activeChannelID = %q, want none", a.activeChannelID)
	}
	// The sidebar still paints and the overlay still dismisses.
	if a.bootstrap.IsLoading() || len(a.sidebar.Items()) != 2 {
		t.Error("sidebar paint must not depend on a restored channel")
	}
}

func TestWorkspaceCachedIgnoredAfterReady(t *testing.T) {
	a := newHarnessApp(t, withWorkspace("T9",
		sidebar.ChannelItem{ID: "C9", Name: "real", Type: "channel"}))
	_, cmd := a.Update(cachedWorkspaceFixture())
	if a.activeTeamID != "T9" {
		t.Errorf("cached msg stole active workspace: activeTeamID = %q", a.activeTeamID)
	}
	if items := a.sidebar.Items(); len(items) != 1 || items[0].ID != "C9" {
		t.Errorf("cached msg replaced the authoritative sidebar: %v", items)
	}
	if msgs := drainCmds(cmd); len(msgs) != 0 {
		t.Errorf("late cached msg must be inert, got %v", msgs)
	}
}

func TestWorkspaceCachedSkipsSelectionForStartupLink(t *testing.T) {
	a := cachedBootHarness(t, nil, nil)
	a.SetStartupLink("C7", "1700000009.000000", "")
	_, cmd := a.Update(cachedWorkspaceFixture())
	if msgs := drainCmds(cmd); len(msgs) != 0 {
		t.Errorf("startup link owns the first selection, got %v", msgs)
	}
	if a.startupLinkNav == nil {
		t.Error("startup nav must stay armed for the ready path")
	}
}

// TestWorkspaceReadyReconcilesProvisional: the authoritative ready for
// the provisionally painted workspace keeps the channel the user is on
// (including one they navigated to mid-boot), does not blank the pane,
// re-fires the selection so the now-live tier dispatch verifies it, and
// clears the pill.
func TestWorkspaceReadyReconcilesProvisional(t *testing.T) {
	cached := []messages.MessageItem{{TS: "1700000001.000000", Text: "from cache"}}
	var fetches []string
	a := cachedBootHarness(t, cached, &fetches)

	_, cmd := a.Update(cachedWorkspaceFixture())
	sel, _ := findChannelSelected(cmd())
	_, _ = a.Update(sel)
	// User navigates during the boot window.
	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	fetches = nil

	_, cmd = a.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		TeamName:      "Acme",
		InitialActive: true,
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "C2", Name: "random", Type: "channel"},
		},
		LastChannelID: "C2",
	})

	if a.messagepane.IsLoading() || a.messagepane.Messages() == nil {
		t.Error("ready path blanked the provisional pane")
	}
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != "C1" {
		t.Fatalf("want the user's mid-boot channel C1 re-selected, got ok=%v id=%q", ok, sel.ID)
	}
	if a.provisionalTeamID != "" {
		t.Error("provisional state not cleared by the authoritative ready")
	}
	// Re-running the selection dispatches the real verify fetch the
	// provisional selection could not.
	_, cmd = a.Update(sel)
	drainCmds(cmd)
	if len(fetches) != 1 || fetches[0] != "C1" {
		t.Errorf("reconciled selection fetched %v, want one verify fetch for C1", fetches)
	}
	// Once the verify lands, nothing is provisional or in flight: the
	// pill clears for good.
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: cached})
	if strings.Contains(a.statusbar.View(120), "cached · syncing") {
		t.Error("pill still up after the authoritative verify landed")
	}
}

func TestWorkspaceReadyOtherTeamReplacesProvisional(t *testing.T) {
	a := cachedBootHarness(t, nil, nil)
	_, cmd := a.Update(cachedWorkspaceFixture())
	if sel, ok := findChannelSelected(cmd()); ok {
		_, _ = a.Update(sel)
	}

	_, cmd = a.Update(WorkspaceReadyMsg{
		TeamID:        "T2",
		TeamName:      "Other",
		InitialActive: true,
		Channels:      []sidebar.ChannelItem{{ID: "C9", Name: "other-general", Type: "channel"}},
	})

	if a.activeTeamID != "T2" {
		t.Errorf("activeTeamID = %q, want the real first-ready workspace T2", a.activeTeamID)
	}
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != "C9" {
		t.Errorf("want T2's channel selected via the default path, got ok=%v id=%q", ok, sel.ID)
	}
	if a.provisionalTeamID != "" {
		t.Error("provisional state must clear when another workspace claims active")
	}
}

func TestWorkspaceFailedClearsProvisionalPill(t *testing.T) {
	a := cachedBootHarness(t, nil, nil)
	_, _ = a.Update(cachedWorkspaceFixture())
	if !strings.Contains(a.statusbar.View(120), "cached · syncing") {
		t.Fatal("precondition: pill up after provisional paint")
	}
	_, _ = a.Update(WorkspaceFailedMsg{TeamName: "Acme"})
	if strings.Contains(a.statusbar.View(120), "cached · syncing") {
		t.Error("pill still claims syncing after the provisional workspace failed to connect")
	}
}

// TestCachedSelectionDrawsUnreadLine: cache-rendered tiers never see a
// MessagesLoadedMsg, so the selection itself must push the persisted
// last-read watermark into the pane.
func TestCachedSelectionDrawsUnreadLine(t *testing.T) {
	cached := []messages.MessageItem{{TS: "1700000002.000000", Text: "newer"}}
	a := cachedBootHarness(t, cached, nil)
	a.SetReadStateReader(func() map[string]cache.ReadState {
		return map[string]cache.ReadState{"C2": {LastReadTS: "1700000001.000000", HasUnread: true}}
	})

	_, cmd := a.Update(cachedWorkspaceFixture())
	sel, _ := findChannelSelected(cmd())
	_, _ = a.Update(sel)

	if got := a.messagepane.LastReadTS(); got != "1700000001.000000" {
		t.Errorf("pane LastReadTS = %q, want the persisted watermark", got)
	}

	// Switching to a channel with no recorded state clears the
	// watermark — C2's line must not survive into C1.
	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	if got := a.messagepane.LastReadTS(); got != "" {
		t.Errorf("pane LastReadTS = %q after switching to a stateless channel, want cleared", got)
	}
}
