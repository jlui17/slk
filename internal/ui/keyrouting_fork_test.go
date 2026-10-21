// internal/ui/keyrouting_fork_test.go
//
// Update-path key routing: real tea.KeyPressMsg fed through App.Update
// (not the mode_handlers_helpers_test.go shims), so the full chain —
// reducer pass-through, handleKey, dispatchModeKey, per-mode handler —
// is what these tests exercise.
package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/sidebar"
)

func pressKeys(a *App, msgs ...tea.KeyPressMsg) {
	for _, m := range msgs {
		_, _ = a.Update(m)
	}
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestKeyRouting_InsertModeEntryAndTyping(t *testing.T) {
	a := newHarnessApp(t)
	pressKeys(a, keyRune('i'))
	if a.mode != ModeInsert {
		t.Fatalf("mode after i = %v, want ModeInsert", a.mode)
	}
	if a.focusedPanel != PanelMessages {
		t.Fatalf("focusedPanel after i = %v, want PanelMessages", a.focusedPanel)
	}
	pressKeys(a, keyRune('h'), keyRune('e'), keyRune('y'))
	if got := a.compose.Value(); got != "hey" {
		t.Fatalf("compose value = %q, want %q", got, "hey")
	}
}

func TestKeyRouting_EscExitsInsertMode(t *testing.T) {
	a := newHarnessApp(t)
	pressKeys(a, keyRune('i'), keyRune('x'))
	pressKeys(a, tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.mode != ModeNormal {
		t.Fatalf("mode after esc = %v, want ModeNormal", a.mode)
	}
	// Keys must route to normal mode again, not the compose box.
	pressKeys(a, keyRune('z'))
	if got := a.compose.Value(); got != "x" {
		t.Fatalf("compose value after esc = %q, want %q (z must not reach compose)", got, "x")
	}
}

func TestKeyRouting_CommandModeTyping(t *testing.T) {
	a := newHarnessApp(t)
	pressKeys(a, keyRune(':'))
	if a.mode != ModeCommand {
		t.Fatalf("mode after : = %v, want ModeCommand", a.mode)
	}
	pressKeys(a, keyRune('w'), keyRune('s'))
	if a.cmdline != "ws" {
		t.Fatalf("cmdline = %q, want %q", a.cmdline, "ws")
	}
	pressKeys(a, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if a.cmdline != "w" {
		t.Fatalf("cmdline after backspace = %q, want %q", a.cmdline, "w")
	}
	pressKeys(a, tea.KeyPressMsg{Code: tea.KeyEscape})
	if a.mode != ModeNormal {
		t.Fatalf("mode after esc = %v, want ModeNormal", a.mode)
	}
	if a.cmdline != "" {
		t.Fatalf("cmdline after esc = %q, want empty", a.cmdline)
	}
}

func TestKeyRouting_TabCyclesFocus(t *testing.T) {
	a := newHarnessApp(t)
	if a.focusedPanel != PanelSidebar {
		t.Fatalf("initial focusedPanel = %v, want PanelSidebar", a.focusedPanel)
	}
	pressKeys(a, tea.KeyPressMsg{Code: tea.KeyTab})
	if a.focusedPanel != PanelMessages {
		t.Fatalf("focusedPanel after tab = %v, want PanelMessages", a.focusedPanel)
	}
	pressKeys(a, tea.KeyPressMsg{Code: tea.KeyTab})
	if a.focusedPanel != PanelSidebar {
		t.Fatalf("focusedPanel after 2x tab = %v, want PanelSidebar", a.focusedPanel)
	}
}

func TestKeyRouting_SidebarJKMovesSelection(t *testing.T) {
	a := newHarnessApp(t,
		withWorkspace("T1",
			sidebar.ChannelItem{ID: "C1", Name: "general", Type: "channel"},
			sidebar.ChannelItem{ID: "C2", Name: "ops", Type: "channel"},
		),
		withApp(func(a *App) { a.sidebar.SelectByID("C1") }),
	)
	pressKeys(a, keyRune('j'))
	if got := a.sidebar.SelectedID(); got != "C2" {
		t.Fatalf("SelectedID after j = %q, want C2", got)
	}
	pressKeys(a, keyRune('k'))
	if got := a.sidebar.SelectedID(); got != "C1" {
		t.Fatalf("SelectedID after k = %q, want C1", got)
	}
}

func TestKeyRouting_CtrlBTogglesSidebar(t *testing.T) {
	a := newHarnessApp(t)
	pressKeys(a, tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	if a.sidebarVisible {
		t.Fatal("sidebarVisible after ctrl+b = true, want false")
	}
	if a.focusedPanel != PanelMessages {
		t.Fatalf("focusedPanel after hiding sidebar = %v, want PanelMessages", a.focusedPanel)
	}
	pressKeys(a, tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl})
	if !a.sidebarVisible {
		t.Fatal("sidebarVisible after 2x ctrl+b = false, want true")
	}
}

// gg in the thread panel selects the parent, the topmost row, and
// scrolls it back into view, zoomed or side by side.
func TestKeyRouting_GGSelectsThreadParent(t *testing.T) {
	for _, zoomed := range []bool{true, false} {
		t.Run(fmt.Sprintf("zoomed=%v", zoomed), func(t *testing.T) {
			a := newHarnessApp(t,
				withHarnessSize(200, 30),
				withApp(func(a *App) {
					parent := messages.MessageItem{TS: "1.0", UserID: "U1", UserName: "alice", Text: "thread-parent-text"}
					var replies []messages.MessageItem
					for i := 0; i < 30; i++ {
						replies = append(replies, messages.MessageItem{TS: fmt.Sprintf("2.%02d", i), UserID: "U2", UserName: "bob", Text: fmt.Sprintf("reply %d", i)})
					}
					a.threadPanel.SetThread(parent, replies, "C1", "1.0")
					a.threadVisible = true
					a.threadFullscreen = zoomed
					a.focusedPanel = PanelThread
				}),
			)
			if strings.Contains(a.View().Content, "thread-parent-text") {
				t.Fatal("precondition: the parent should be scrolled out of view on open")
			}

			pressKeys(a, keyRune('g'), keyRune('g'))

			if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != "1.0" {
				t.Errorf("SelectedReply after gg = %+v, want the parent (TS 1.0)", sel)
			}
			if !strings.Contains(a.View().Content, "thread-parent-text") {
				t.Error("parent not visible after gg: the thread did not scroll to the top")
			}
		})
	}
}
