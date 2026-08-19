// internal/ui/app_threadzoom_test.go
//
// The `z` thread zoom: layout geometry and hit-test bands while the
// thread owns the whole messages area, the toggle's state transitions,
// and the focus cycle with the messages pane no longer drawn.
package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// zoomTestApp builds an App with a thread open, wide enough that the
// side-by-side layout clears both pane minimums (a narrower terminal
// auto-hides the thread instead).
func zoomTestApp() *App {
	a := NewApp()
	a.width = 200
	a.height = 50
	a.sidebarVisible = true
	a.threadVisible = true
	return a
}

// zoomFrame recomputes the layout the way App.View does.
func zoomFrame(a *App) panelLayoutFrame {
	return a.layout.Compute(a.width, a.height, a.workspaceRail.Width(), a.sidebar.Width(), a.sidebarVisible, a.threadVisible, a.threadFullscreen)
}

func TestZoomLayoutThreadSpansMessagesArea(t *testing.T) {
	a := zoomTestApp()
	side := zoomFrame(a)
	msgArea := side.MsgWidth + side.MsgBorder + side.ThreadWidth + side.ThreadBorder

	a.threadFullscreen = true
	zoom := zoomFrame(a)

	if got := zoom.ThreadWidth + zoom.ThreadBorder; got != msgArea {
		t.Errorf("zoomed thread extent = %d, want the whole messages area %d", got, msgArea)
	}
	if !zoom.ThreadFullscreen {
		t.Error("frame.ThreadFullscreen = false while zoomed")
	}
	// The suppressed messages pane keeps its side-by-side width so its
	// off-screen side effects (compose width, window bounds) don't move.
	if zoom.MsgWidth != side.MsgWidth {
		t.Errorf("MsgWidth while zoomed = %d, want the unzoomed %d", zoom.MsgWidth, side.MsgWidth)
	}
}

func TestZoomLayoutCollapsesMessagesBand(t *testing.T) {
	a := zoomTestApp()
	side := zoomFrame(a)
	msgArea := side.MsgWidth + side.MsgBorder + side.ThreadWidth + side.ThreadBorder
	sidebarEnd := a.layout.SidebarEnd()

	a.threadFullscreen = true
	_ = zoomFrame(a)

	if a.layout.MsgEnd() != sidebarEnd {
		t.Errorf("MsgEnd = %d while zoomed, want it collapsed onto SidebarEnd %d", a.layout.MsgEnd(), sidebarEnd)
	}
	if want := sidebarEnd + msgArea; a.layout.ThreadEnd() != want {
		t.Errorf("ThreadEnd = %d, want %d", a.layout.ThreadEnd(), want)
	}
}

func TestZoomPanelAtRoutesMessagesColumnsToThread(t *testing.T) {
	a := zoomTestApp()
	_ = zoomFrame(a)
	x := a.layout.SidebarEnd() + 5
	if panel, _, _, _ := a.panelAt(x, 5); panel != PanelMessages {
		t.Fatalf("precondition: x=%d should be the messages pane unzoomed, got %v", x, panel)
	}

	a.threadFullscreen = true
	_ = zoomFrame(a)

	panel, px, py, ok := a.panelAt(x, 5)
	if !ok || panel != PanelThread {
		t.Fatalf("x=%d while zoomed: want thread/ok, got panel=%v ok=%v", x, panel, ok)
	}
	if want := x - a.layout.MsgEnd() - 1; px != want {
		t.Errorf("paneX = %d, want %d", px, want)
	}
	if py != 4 {
		t.Errorf("paneY = %d, want 4", py)
	}
}

func TestZoomLayoutBypassesAutoHide(t *testing.T) {
	a := zoomTestApp()
	a.sidebarVisible = false
	a.width = 70 // 35% of the area lands under the 30-col thread minimum

	if f := zoomFrame(a); !f.ThreadAutoHidden {
		t.Fatalf("precondition: width=%d should auto-hide the thread side-by-side", a.width)
	}

	a.threadFullscreen = true
	f := zoomFrame(a)
	if f.ThreadAutoHidden {
		t.Error("a zoomed thread has no messages pane to protect; auto-hide must not fire")
	}
	if want := a.width - f.RailWidth; f.ThreadWidth+f.ThreadBorder != want {
		t.Errorf("zoomed thread extent = %d, want %d", f.ThreadWidth+f.ThreadBorder, want)
	}
}

func TestZoomKeyTogglesAndPullsFocusOffMessages(t *testing.T) {
	a := zoomTestApp()
	a.focusedPanel = PanelMessages
	z := tea.KeyPressMsg{Code: 'z', Text: "z"}

	_ = handleNormalMode(a, z)
	if !a.threadFullscreen {
		t.Fatal("z with a thread open should zoom it")
	}
	if a.focusedPanel != PanelThread {
		t.Errorf("focusedPanel = %v after zooming, want PanelThread", a.focusedPanel)
	}

	_ = handleNormalMode(a, z)
	if a.threadFullscreen {
		t.Error("a second z should restore the side-by-side layout")
	}
}

func TestZoomKeyNoOpWithoutThread(t *testing.T) {
	a := zoomTestApp()
	a.threadVisible = false
	a.focusedPanel = PanelMessages

	_ = handleNormalMode(a, tea.KeyPressMsg{Code: 'z', Text: "z"})

	if a.threadFullscreen {
		t.Error("z with no thread open should not zoom")
	}
	if a.focusedPanel != PanelMessages {
		t.Errorf("focusedPanel = %v, want it untouched", a.focusedPanel)
	}
}

func TestZoomResetsOnEveryCloseThreadPath(t *testing.T) {
	cases := []struct {
		name  string
		close func(*App)
	}{
		{"CloseThread", func(a *App) { a.CloseThread() }},
		{"ToggleThread", func(a *App) { a.ToggleThread() }},
		{"q", func(a *App) { _ = handleNormalMode(a, tea.KeyPressMsg{Code: 'q', Text: "q"}) }},
		{"esc", func(a *App) { _ = handleNormalMode(a, tea.KeyPressMsg{Code: tea.KeyEscape}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := zoomTestApp()
			a.threadFullscreen = true
			a.focusedPanel = PanelThread

			tc.close(a)

			if a.threadVisible {
				t.Fatal("thread should be closed")
			}
			if a.threadFullscreen {
				t.Error("closing the thread must reset the zoom so a reopened thread starts unzoomed")
			}
		})
	}
}

func TestZoomFocusCycleSkipsMessages(t *testing.T) {
	a := zoomTestApp()
	a.threadFullscreen = true
	a.focusedPanel = PanelThread

	a.FocusNext()
	if a.focusedPanel != PanelSidebar {
		t.Fatalf("FocusNext from thread = %v, want PanelSidebar", a.focusedPanel)
	}
	a.FocusNext()
	if a.focusedPanel != PanelThread {
		t.Fatalf("FocusNext from sidebar = %v, want PanelThread (messages is not drawn)", a.focusedPanel)
	}
	a.FocusPrev()
	if a.focusedPanel != PanelSidebar {
		t.Fatalf("FocusPrev from thread = %v, want PanelSidebar", a.focusedPanel)
	}

	a.sidebarVisible = false
	a.focusedPanel = PanelThread
	a.FocusNext()
	if a.focusedPanel != PanelThread {
		t.Errorf("FocusNext with only the thread visible = %v, want PanelThread", a.focusedPanel)
	}
}

func TestZoomSuppressesMessagesRegionAndWidensThread(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a.threadVisible = true
	side := zoomFrame(a)
	msgArea := side.MsgWidth + side.MsgBorder + side.ThreadWidth + side.ThreadBorder

	a.threadFullscreen = true
	frame := zoomFrame(a)

	if got := a.renderWindowsRegion(frame, 0, false); got != "" {
		t.Errorf("messages region must render empty while zoomed, got %d cols", lipgloss.Width(got))
	}
	threadRegion := a.renderThreadRegion(frame, 0)
	row, _, _ := strings.Cut(threadRegion, "\n")
	if got := lipgloss.Width(row); got != msgArea {
		t.Errorf("zoomed thread region width = %d, want the whole messages area %d", got, msgArea)
	}
}

func TestZoomDropsSixelPlacements(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	a.threadVisible = true
	a.threadFullscreen = true

	// The thread pane is halfblock-only for images, so the hidden
	// windows' sixels must not stay painted over it.
	if got := a.collectSixelPlacements(zoomFrame(a)); len(got) != 0 {
		t.Errorf("collectSixelPlacements while zoomed = %d placements, want 0", len(got))
	}
}
