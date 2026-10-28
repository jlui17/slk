package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// clickConfirmText clicks the screen cell where text is drawn inside the
// open confirm prompt, positioned the way the router centres it.
func clickConfirmText(t *testing.T, a *App, text string) tea.Cmd {
	t.Helper()
	box := a.confirmPrompt.View()
	w, h := lipgloss.Width(box), lipgloss.Height(box)
	startX, startY := max((a.width-w)/2, 0), max((a.height-h)/2, 0)
	for row, line := range strings.Split(ansi.Strip(box), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			return reduceModalClick(a, tea.MouseClickMsg{
				X: startX + ansi.StringWidth(line[:i]) + 1, Y: startY + row, Button: tea.MouseLeft,
			})
		}
	}
	t.Fatalf("%q not in prompt:\n%s", text, box)
	return nil
}

// Review Focus 1: a click on Cancel must never confirm. The router's usual
// "synthesise Enter" would, because Enter is a Confirm key.
func TestQuitPromptClickCancelDoesNotQuit(t *testing.T) {
	a := newTestApp(t, withSize(120, 40))
	a.openQuitConfirm()

	cmd := clickConfirmText(t, a, "[n/Esc] cancel")
	if cmd != nil {
		if _, quit := cmd().(tea.QuitMsg); quit {
			t.Fatal("clicking Cancel quit slk")
		}
	}
	if a.confirmPrompt.IsVisible() || a.mode != ModeNormal {
		t.Errorf("after Cancel: visible=%v mode=%v, want hidden and normal", a.confirmPrompt.IsVisible(), a.mode)
	}
}

func TestQuitPromptClickConfirmQuits(t *testing.T) {
	a := newTestApp(t, withSize(120, 40))
	a.openQuitConfirm()

	cmd := clickConfirmText(t, a, "[y] confirm")
	if cmd == nil {
		t.Fatal("clicking Confirm returned no command")
	}
	if _, quit := cmd().(tea.QuitMsg); !quit {
		t.Error("clicking Confirm should quit")
	}
	if a.mode != ModeNormal {
		t.Errorf("mode = %v, want normal after the prompt closed", a.mode)
	}
}

func TestConfirmPromptClickOffButtonDoesNothing(t *testing.T) {
	a := newTestApp(t, withSize(120, 40))
	a.openQuitConfirm()

	if cmd := clickConfirmText(t, a, "Quit slk?"); cmd != nil {
		t.Error("a click on the title should do nothing")
	}
	if !a.confirmPrompt.IsVisible() || a.mode != ModeConfirm {
		t.Error("a click on the title should leave the prompt open")
	}
}

// The host must set both button styles; unset styles render plain text.
func TestConfirmPromptButtonStylesAreThemed(t *testing.T) {
	s := confirmPromptStyles()
	if s.Button.Render("x") == "x" || s.ButtonActive.Render("x") == "x" {
		t.Error("the theme must style both buttons")
	}
	if s.Button.Render("x") == s.ButtonActive.Render("x") {
		t.Error("the highlighted button must look different")
	}
}
