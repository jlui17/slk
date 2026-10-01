// internal/ui/compose_preview_focus_fork.go
//
// Keyboard focus on the thumbnail row of a compose box (see
// compose_preview_fork.go): Up from the first row of the text selects
// the last thumbnail, and while one is selected every key belongs to
// the row, so Enter views the image instead of sending the message.
package ui

import (
	"bytes"
	"image"
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/compose"
)

// pasteThumbFocus is which thumbnail has the keyboard focus, if any.
// Nothing invalidates it: selectedPasteThumb checks it against the
// current mode, the focused box and the last drawn frame on every use.
type pasteThumbFocus struct {
	// box is the compose box whose thumbnail is selected; nil while the
	// text has the focus.
	box *compose.Model
	// index is the selected thumbnail, counted over the drawn ones.
	index int
	// drawn is, per compose box, the attachment index of each thumbnail
	// the last frame drew.
	drawn map[*compose.Model][]int
}

// focusedCompose is the compose box insert mode types into.
func (a *App) focusedCompose() *compose.Model {
	if a.focusedPanel == PanelThread && a.threadVisible {
		return &a.threadCompose
	}
	return &a.compose
}

// pasteThumbRowReachable reports whether Up from m's text may move to
// its thumbnail row: pickers and an upload keep their keys.
func (a *App) pasteThumbRowReachable(m *compose.Model) bool {
	return a.mode == ModeInsert && m == a.focusedCompose() && !m.Uploading() &&
		!m.IsEmojiActive() && !m.IsMentionActive() && !m.IsChannelActive()
}

// selectedPasteThumb returns the selected thumbnail of m, or -1. The
// focus drops back to the text as soon as it stops describing what is
// on screen: insert mode left, another box focused, or the thumbnail
// no longer drawn.
func (a *App) selectedPasteThumb(m *compose.Model) int {
	f := &a.pasteThumbFocus
	if f.box == nil {
		return -1
	}
	if a.mode != ModeInsert || f.box != a.focusedCompose() || f.index >= len(f.drawn[f.box]) {
		f.box = nil
	}
	if f.box != m {
		return -1
	}
	return f.index
}

// handlePasteThumbKey is insert mode's first stop. It takes Up when
// that moves the focus to the thumbnail row, and every key while a
// thumbnail has the focus.
func (a *App) handlePasteThumbKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	m := a.focusedCompose()
	f := &a.pasteThumbFocus
	code := msg.Key().Code
	mod := msg.Key().Mod &^ (tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock)

	sel := a.selectedPasteThumb(m)
	if sel < 0 {
		if code == tea.KeyUp && mod == 0 && len(f.drawn[m]) > 0 &&
			a.pasteThumbRowReachable(m) && m.CursorOnFirstVisualRow() {
			f.box, f.index = m, len(f.drawn[m])-1
			return nil, true
		}
		return nil, false
	}

	drawn := f.drawn[m]
	switch {
	case key.Matches(msg, a.keys.Escape), code == tea.KeyDown && mod == 0:
		f.box = nil
	case mod != 0:
	case code == tea.KeyLeft, code == 'h':
		f.index = max(sel-1, 0)
	case code == tea.KeyRight, code == 'l':
		f.index = min(sel+1, len(drawn)-1)
	case code == tea.KeyEnter:
		a.openPastedImagePreview(m.Attachments()[drawn[sel]])
	case code == tea.KeyBackspace, code == tea.KeyDelete, code == 'x':
		m.RemoveAttachmentAt(drawn[sel])
		// Keep drawn true until the next frame redraws it: the
		// attachments after the removed one moved down by one.
		drawn = slices.Delete(drawn, sel, sel+1)
		for i := sel; i < len(drawn); i++ {
			drawn[i]--
		}
		f.drawn[m] = drawn
		f.index = max(sel-1, 0)
	}
	return nil, true
}

// openPastedImagePreview shows a pasted image in the full-screen
// preview. It opens with no message behind it and no path, so the
// preview's sibling keys and its open-in-system-viewer key do nothing;
// closing it leaves insert mode and the thumbnail focus as they were.
func (a *App) openPastedImagePreview(att compose.PendingAttachment) {
	img, _, err := image.Decode(bytes.NewReader(att.Bytes))
	if err != nil {
		return
	}
	overlay := imgpkg.NewPreview(imgpkg.PreviewInput{Name: att.Filename, Img: img})
	a.preview.Open(&overlay, "", "", 0)
}
