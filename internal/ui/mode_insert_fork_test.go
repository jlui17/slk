package ui

import (
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
