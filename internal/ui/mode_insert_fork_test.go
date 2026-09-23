package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/compose"
)

// Ctrl+U (what kitty sends for cmd+backspace) falls through to the
// textarea's delete-before-cursor: it kills the current line's text
// and leaves the rest of the draft and any pending attachments alone.
func TestHandleInsertMode_CtrlU_DeletesCurrentLineOnly(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	app.focusedPanel = PanelMessages
	app.SetMode(ModeInsert)
	_ = app.compose.Focus()
	app.compose.SetValue("line1\nline2")
	app.compose.AddAttachment(compose.PendingAttachment{Filename: "a.png", Size: 1})

	// SetValue lands the cursor at the end of the last line.
	app.handleInsertMode(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})

	if got := app.compose.Value(); got != "line1\n" {
		t.Errorf("expected only the current line's text deleted, got %q", got)
	}
	if len(app.compose.Attachments()) != 1 {
		t.Errorf("expected attachments preserved, got %d", len(app.compose.Attachments()))
	}
}

// Ctrl+E (what kitty sends for cmd+right) falls through to the
// textarea's LineEnd instead of opening the external editor.
func TestHandleInsertMode_CtrlE_MovesToLineEnd(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	app.focusedPanel = PanelMessages
	app.SetMode(ModeInsert)
	_ = app.compose.Focus()
	app.compose.SetValue("hello")
	app.compose.MoveCursorToStart()

	app.handleInsertMode(tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl})

	if got := statusbarText(app); strings.Contains(got, "No editor configured") {
		t.Errorf("Ctrl+E reached the editor path, status bar = %q", got)
	}
	// The compose model doesn't export its cursor column; a typed rune
	// lands where the cursor is.
	app.handleInsertMode(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if got := app.compose.Value(); got != "hellox" {
		t.Errorf("expected the cursor at the end of the line, typed rune gave %q", got)
	}
}

// Ctrl+X is the fork's key for upstream's edit-the-draft-in-$EDITOR.
func TestHandleInsertMode_CtrlX_ReachesEditor(t *testing.T) {
	app := NewApp()
	app.activeChannelID = "C1"
	app.focusedPanel = PanelMessages
	app.SetMode(ModeInsert)
	_ = app.compose.Focus()
	app.compose.SetValue("hello")

	app.handleInsertMode(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})

	// No editor is configured on a bare App, so the editor path toasts.
	if got := statusbarText(app); !strings.Contains(got, "No editor configured") {
		t.Errorf("status bar = %q, want the no-editor toast", got)
	}
	if got := app.compose.Value(); got != "hello" {
		t.Errorf("draft changed without an editor: %q", got)
	}
}
