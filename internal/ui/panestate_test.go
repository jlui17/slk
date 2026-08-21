package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

func newPaneStateTestApp(t *testing.T) (*App, *[]paneReport) {
	t.Helper()
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	var got []paneReport
	a.SetPaneStateRecorder(func(teamID, channelID, threadTS string) {
		got = append(got, paneReport{teamID, channelID, threadTS})
	})
	_, _ = a.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		InitialActive: true,
		Channels:      []sidebar.ChannelItem{{ID: "C1", Name: "general", Type: "channel"}},
	})
	return a, &got
}

// TestPaneStateReporting is the headline of the pane-state restore
// feature's write side: every open-state change (channel select, thread
// open, thread close) reaches the recorder exactly once, so the
// persisted row always matches what's on screen.
func TestPaneStateReporting(t *testing.T) {
	a, got := newPaneStateTestApp(t)

	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	_ = a.openThreadPanel(messages.MessageItem{TS: "111.222"}, "C1", "111.222")
	// Replies reloading re-enters setThreadPanel with the same thread;
	// the recorder must not re-fire.
	a.setThreadPanel(messages.MessageItem{TS: "111.222"}, nil, "C1", "111.222")
	a.CloseThread()

	want := []paneReport{
		{"T1", "C1", ""},
		{"T1", "C1", "111.222"},
		{"T1", "C1", ""},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("reports = %v, want %v", *got, want)
	}
}

// TestPaneStateReportingWorkspaceSwitch: reduceWorkspaceSwitched closes
// the thread panel before reassigning activeTeamID, so the close report
// must carry the OLD workspace with the old channel — recording the new
// workspace against the old workspace's channel would persist a channel
// the new workspace doesn't have.
func TestPaneStateReportingWorkspaceSwitch(t *testing.T) {
	a, got := newPaneStateTestApp(t)

	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "general", Type: "channel"})
	_ = a.openThreadPanel(messages.MessageItem{TS: "111.222"}, "C1", "111.222")
	_, _ = a.Update(WorkspaceSwitchedMsg{
		TeamID:   "T2",
		Channels: []sidebar.ChannelItem{{ID: "C9", Name: "other", Type: "channel"}},
	})
	// The switch queues a ChannelSelectedMsg for the new workspace's
	// restored channel; the test harness delivers it by hand.
	_, _ = a.Update(ChannelSelectedMsg{ID: "C9", Name: "other", Type: "channel"})

	want := []paneReport{
		{"T1", "C1", ""},
		{"T1", "C1", "111.222"},
		{"T1", "C1", ""}, // thread close during the switch: still T1
		{"T2", "C9", ""},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("reports = %v, want %v", *got, want)
	}
}

// TestPaneStateReportingSkipsEmptyChannel: CloseThread runs before any
// channel is open at boot; an empty channelID must not reach the
// recorder (it would clobber the persisted state being restored).
func TestPaneStateReportingSkipsEmptyChannel(t *testing.T) {
	a := NewApp()
	called := false
	a.SetPaneStateRecorder(func(teamID, channelID, threadTS string) { called = true })
	a.CloseThread()
	if called {
		t.Error("recorder called with no channel open")
	}
}
