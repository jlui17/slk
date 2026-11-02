package ui

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/gammons/slk/internal/ui/workspace"
)

// railRefreshApp builds an App with T1 active and T2's dot lit, then
// re-points the rail reader so T2 is dark. The title still says "+1"
// because nothing has recomputed it; a message that reaches
// notifyReadStateChanged drops the "+1", one that does not leaves it.
// That is the same observation TestWorkspaceReady_RefreshesRailAndTitle
// makes for #207's refresh sites.
func railRefreshApp(t *testing.T) *App {
	t.Helper()
	app := setupAppForTitleTest(t,
		[]sidebar.ChannelItem{{ID: "C1", Name: "general", Type: "channel"}},
		[]workspace.WorkspaceItem{
			{ID: "T1", Name: "SWAP", Initials: "SW"},
			{ID: "T2", Name: "Other", Initials: "OT"},
		},
		map[string]cache.ReadState{},
		nil,
	)
	unreads := []string{"T2"}
	// #219 renamed App.SetWorkspaceUnreadReader to SetUnreadService; the
	// rail reader is the part this test exercises, so install it directly.
	app.workspaceRail.SetUnreadReader(func() []string { return unreads })
	app.activeTeamID = "T1"
	app.notifyReadStateChanged()
	if got, want := app.windowTitle, "slk SW +1"; got != want {
		t.Fatalf("precondition: windowTitle = %q want %q", got, want)
	}
	unreads = nil
	return app
}

// TestThreadsBadgeUpdated_RefreshesRail pins that Slack's thread state
// recomputes the rail, for the workspace on screen and for one in the
// background: the rail's thread half (cmd/slk railUnreadWorkspaces)
// reads the has_unreads stored before this message was sent, and
// nothing else on the thread paths recomputes it.
func TestThreadsBadgeUpdated_RefreshesRail(t *testing.T) {
	for _, team := range []string{"T1", "T2"} {
		t.Run(team, func(t *testing.T) {
			app := railRefreshApp(t)
			app.Update(ThreadsBadgeUpdatedMsg{TeamID: team, UnreadThreads: 0})
			if got, want := app.windowTitle, "slk SW"; got != want {
				t.Errorf("windowTitle = %q want %q: the rail was not recomputed", got, want)
			}
		})
	}
}
