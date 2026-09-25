// internal/ui/app_threadzoom_fork_test.go
//
// A thread opened from the messages pane starts zoomed when the
// side-by-side layout would leave its panel cramped.
package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// crampedTestApp builds an App at the given terminal width, sidebar
// visible, focused on a messages pane whose selected message has a
// thread.
func crampedTestApp(t *testing.T, width int) *App {
	t.Helper()
	return newHarnessApp(t,
		withHarnessSize(width, 50),
		withHarnessMessages(
			messages.MessageItem{TS: "1.0", UserName: "alice", UserID: "U1", Text: "parent", Timestamp: "1:00 PM", ReplyCount: 3},
		),
		withApp(func(a *App) {
			a.activeChannelID = "C1"
			a.focusedPanel = PanelMessages
			a.setThreadFetcherForTest(func(_ ids.ChannelID, threadTS ids.ThreadTS) core.Msg {
				return ThreadRepliesLoadedMsg{ThreadTS: string(threadTS), Replies: nil}
			})
		}),
	)
}

// sideBySideThreadWidth is the thread panel width the layout gives an
// open, unzoomed thread, and whether it would auto-hide instead.
func sideBySideThreadWidth(a *App) (width int, autoHidden bool) {
	f := newPanelLayout().Compute(a.width, a.height, a.workspaceRail.Width(), a.sidebar.Width(), a.sidebarVisible, true, false)
	return f.ThreadWidth, f.ThreadAutoHidden
}

func pressEnter(a *App) {
	_, _ = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
}

func TestOpenThreadZoomsWhenSideBySideWouldAutoHide(t *testing.T) {
	a := crampedTestApp(t, 88)
	if _, autoHidden := sideBySideThreadWidth(a); !autoHidden {
		t.Fatalf("precondition: width=%d should auto-hide a side-by-side thread", a.width)
	}

	pressEnter(a)

	if !a.threadVisible {
		t.Fatal("threadVisible = false after Enter; want true")
	}
	if !a.threadFullscreen {
		t.Error("threadFullscreen = false after Enter in a narrow terminal; want the thread opened zoomed")
	}
	if a.focusedPanel != PanelThread {
		t.Errorf("focusedPanel = %v, want PanelThread", a.focusedPanel)
	}

	_ = a.View()
	if !a.threadVisible {
		t.Error("View auto-hid the thread; a zoomed thread must survive the render")
	}
}

func TestOpenThreadStaysSideBySideWhenRoomy(t *testing.T) {
	a := crampedTestApp(t, 200)
	if w, autoHidden := sideBySideThreadWidth(a); autoHidden || w < crampedThreadWidth {
		t.Fatalf("precondition: width=%d should give a roomy thread panel, got %d cols (autoHidden=%v)", a.width, w, autoHidden)
	}

	pressEnter(a)

	if !a.threadVisible {
		t.Fatal("threadVisible = false after Enter; want true")
	}
	if a.threadFullscreen {
		t.Error("threadFullscreen = true after Enter in a wide terminal; want side by side")
	}
}

func TestOpenThreadZoomedByCrampTogglesBackToSideBySide(t *testing.T) {
	// Wide enough that the side-by-side thread clears the layout's
	// auto-hide minimums, narrow enough that it is still cramped, so
	// the unzoomed state survives a render.
	a := crampedTestApp(t, 140)
	if w, autoHidden := sideBySideThreadWidth(a); autoHidden || w >= crampedThreadWidth {
		t.Fatalf("precondition: width=%d should give a cramped but visible thread panel, got %d cols (autoHidden=%v)", a.width, w, autoHidden)
	}

	pressEnter(a)
	if !a.threadFullscreen {
		t.Fatal("precondition: a cramped thread should open zoomed")
	}

	_, _ = a.Update(tea.KeyPressMsg{Code: 't', Text: "t"})
	if a.threadFullscreen {
		t.Error("t after a zoomed open should restore the side-by-side layout")
	}

	_ = a.View()
	if !a.threadVisible {
		t.Error("the side-by-side thread should stay visible at this width")
	}
	if a.threadFullscreen {
		t.Error("threadFullscreen flipped back on during render")
	}
}
