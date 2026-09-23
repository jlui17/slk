package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

func newAsyncPasteApp(t *testing.T, image, text []byte) *App {
	t.Helper()
	app := NewApp()
	app.activeChannelID = "C1"
	app.focusedPanel = PanelMessages
	app.SetMode(ModeInsert)
	// The textarea ignores input when blurred; insert mode focuses it
	// in the real app.
	_ = app.compose.Focus()
	app.SetAsyncClipboardReader(func(f core.ClipboardFormat) []byte {
		if f == core.ClipboardImage {
			return image
		}
		return text
	})
	return app
}

func statusbarToast(app *App) string { return app.statusbar.View(120) }

// runSnapshot executes the background read cmd and feeds its message
// back through Update, like the Bubble Tea runtime would.
func runSnapshot(t *testing.T, app *App, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a background read cmd, got nil")
	}
	msg := cmd()
	if _, ok := msg.(clipboardSnapshotMsg); !ok {
		t.Fatalf("expected clipboardSnapshotMsg, got %T", msg)
	}
	_, next := app.Update(msg)
	return next
}

func TestAsyncPaste_CtrlV_ShowsToastThenAttachesImage(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake")
	app := newAsyncPasteApp(t, pngBytes, nil)

	cmd := app.handleInsertMode(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})

	if len(app.compose.Attachments()) != 0 {
		t.Fatal("attachment must not land before the background read returns")
	}
	if !strings.Contains(statusbarToast(app), pastingToast) {
		t.Fatalf("expected %q in the status bar, got %q", pastingToast, statusbarToast(app))
	}

	runSnapshot(t, app, cmd)

	atts := app.compose.Attachments()
	if len(atts) != 1 || atts[0].Mime != "image/png" {
		t.Fatalf("expected one image/png attachment after the read, got %+v", atts)
	}
	if strings.Contains(statusbarToast(app), pastingToast) {
		t.Fatalf("pasting toast must clear once the read lands, got %q", statusbarToast(app))
	}
	if app.asyncPaste.inFlight {
		t.Fatal("inFlight must reset after the snapshot is applied")
	}
}

func TestAsyncPaste_CtrlV_WhileInFlight_IsSwallowed(t *testing.T) {
	app := newAsyncPasteApp(t, nil, []byte("hello"))

	first := app.handleInsertMode(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	second := app.handleInsertMode(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	if second != nil {
		t.Fatal("a second Ctrl+V during an in-flight read must not start another")
	}

	runSnapshot(t, app, first)
	if got := app.compose.Value(); got != "hello" {
		t.Fatalf("expected clipboard text pasted once, got %q", got)
	}
}

func TestAsyncPaste_BracketedPaste_TextLandsAfterRead(t *testing.T) {
	app := newAsyncPasteApp(t, nil, nil)

	_, cmd := app.Update(tea.PasteMsg{Content: "typed elsewhere"})

	if app.compose.Value() != "" {
		t.Fatalf("text must wait for the clipboard check, got %q", app.compose.Value())
	}
	if !strings.Contains(statusbarToast(app), pastingToast) {
		t.Fatalf("expected %q in the status bar, got %q", pastingToast, statusbarToast(app))
	}

	runSnapshot(t, app, cmd)

	if got := app.compose.Value(); got != "typed elsewhere" {
		t.Fatalf("expected the bracketed text in the compose, got %q", got)
	}
	if strings.Contains(statusbarToast(app), pastingToast) {
		t.Fatal("pasting toast must clear once the read lands")
	}
}

func TestAsyncPaste_BracketedPaste_ImageWins(t *testing.T) {
	pngBytes := []byte("\x89PNG\r\n\x1a\nfake")
	app := newAsyncPasteApp(t, pngBytes, nil)

	_, cmd := app.Update(tea.PasteMsg{Content: "irrelevant"})
	runSnapshot(t, app, cmd)

	if len(app.compose.Attachments()) != 1 {
		t.Fatalf("expected the clipboard image attached, got %d attachments", len(app.compose.Attachments()))
	}
	if app.compose.Value() != "" {
		t.Fatalf("bracketed text must be dropped when an image attaches, got %q", app.compose.Value())
	}
}

func TestAsyncPaste_ReadLandsAfterLeavingInsert_IsDropped(t *testing.T) {
	app := newAsyncPasteApp(t, nil, []byte("late"))

	cmd := app.handleInsertMode(tea.KeyPressMsg{Code: 'v', Mod: tea.ModCtrl})
	app.SetMode(ModeNormal)
	runSnapshot(t, app, cmd)

	if app.compose.Value() != "" {
		t.Fatalf("a read that lands outside insert mode must not paste, got %q", app.compose.Value())
	}
	if strings.Contains(statusbarToast(app), pastingToast) {
		t.Fatal("pasting toast must clear even when the paste is dropped")
	}
}
