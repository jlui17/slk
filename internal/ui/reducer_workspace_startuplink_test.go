package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/sidebar"
)

func findToast(msg tea.Msg) (ToastMsg, bool) {
	if toast, ok := msg.(ToastMsg); ok {
		return toast, true
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if toast, ok := findToast(c()); ok {
				return toast, true
			}
		}
	}
	return ToastMsg{}, false
}

// TestWorkspaceReadyStartupLink is the headline of `slk <permalink>`:
// a startup link queued via SetStartupLink overrides the last-visited
// restore on the InitialActive workspace, opening the linked channel
// and arming pendingLinkNav so the reducer_channels completion hooks
// finish the message-select / thread-open.
func TestWorkspaceReadyStartupLink(t *testing.T) {
	ready := WorkspaceReadyMsg{
		TeamID:        "T1",
		InitialActive: true,
		Channels: []sidebar.ChannelItem{
			{ID: "C1", Name: "general", Type: "channel"},
			{ID: "C2", Name: "random", Type: "channel"},
		},
		FinderItems: []channelfinder.Item{
			{ID: "C1", Name: "general", Type: "channel", Joined: true},
			{ID: "C3", Name: "browseable", Type: "channel", Joined: false},
		},
		LastChannelID: "C2",
	}
	newAppWithLink := func(channelID string) *App {
		a := NewApp()
		_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
		a.SetStartupLink(channelID, "1779284733.270139", "1779284000.000100")
		return a
	}

	// Linked sidebar channel wins over LastChannelID; nav is armed.
	a := newAppWithLink("C1")
	_, cmd := a.Update(ready)
	if sel, ok := findChannelSelected(cmd()); !ok || sel.ID != "C1" {
		t.Errorf("link to C1: want C1 opened, got ok=%v id=%q", ok, sel.ID)
	}
	if a.pendingLinkNav == nil || a.pendingLinkNav.channelID != "C1" ||
		a.pendingLinkNav.messageTS != "1779284733.270139" ||
		a.pendingLinkNav.threadTS != "1779284000.000100" {
		t.Errorf("link to C1: pendingLinkNav not armed, got %+v", a.pendingLinkNav)
	}
	if a.startupLinkNav != nil {
		t.Error("startupLinkNav not consumed")
	}

	// Finder-only (browseable, unjoined) channels resolve too, matching
	// the in-app router's ChannelService.Lookup scan.
	a = newAppWithLink("C3")
	_, cmd = a.Update(ready)
	if sel, ok := findChannelSelected(cmd()); !ok || sel.ID != "C3" {
		t.Errorf("link to C3: want C3 opened, got ok=%v id=%q", ok, sel.ID)
	}

	// Unknown channel: fall back to the last-visited restore, drop the
	// nav, and surface a toast.
	a = newAppWithLink("CGONE")
	_, cmd = a.Update(ready)
	msg := cmd()
	if sel, ok := findChannelSelected(msg); !ok || sel.ID != "C2" {
		t.Errorf("unknown link channel: want C2 fallback, got ok=%v id=%q", ok, sel.ID)
	}
	if a.pendingLinkNav != nil {
		t.Errorf("unknown link channel: pendingLinkNav should stay nil, got %+v", a.pendingLinkNav)
	}
	if _, ok := findToast(msg); !ok {
		t.Error("unknown link channel: want a toast")
	}
}

// TestWorkspaceReadyStartupLinkOnlyInitialActive pins that a
// non-InitialActive WorkspaceReadyMsg leaves the startup link queued:
// main.go makes the link's workspace the initial active one, and other
// workspaces becoming ready first must not consume or trigger it.
func TestWorkspaceReadyStartupLinkOnlyInitialActive(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	a.SetStartupLink("C1", "1779284733.270139", "")
	_, _ = a.Update(WorkspaceReadyMsg{
		TeamID: "T2",
		Channels: []sidebar.ChannelItem{
			{ID: "C9", Name: "other", Type: "channel"},
		},
	})
	if a.startupLinkNav == nil {
		t.Error("non-initial workspace consumed the startup link")
	}
	if a.pendingLinkNav != nil {
		t.Errorf("non-initial workspace armed pendingLinkNav: %+v", a.pendingLinkNav)
	}
}
