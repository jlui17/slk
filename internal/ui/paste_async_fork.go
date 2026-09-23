package ui

import (
	"sync"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
)

// clipboardReader reads the clipboard in one format; nil means nothing
// of that kind is on it. Same shape as core.DesktopService.ReadClipboard.
type clipboardReader func(f core.ClipboardFormat) []byte

// pastingToast is the status-bar toast shown while a clipboard read is
// in flight. Text, not a spinner: the read finishes in well under a
// second, and the toast slot has no live tick of its own.
const pastingToast = "Pasting from clipboard…"

// asyncPasteState makes smart paste non-blocking for a clipboard reader
// that is slow enough to notice (the host bridge in clipboard_remote.go:
// one osascript launch per read). Native reads are instant, so they
// stay on the synchronous upstream path; SetAsyncClipboardReader turns
// this on.
//
// Flow: the Ctrl+V / bracketed-paste hook calls pasteAsync, which shows
// pastingToast and returns a command that reads image and text off the
// UI goroutine. Its clipboardSnapshotMsg then replays the exact upstream
// paste (smartPaste or reducePaste) against a reader that serves the
// snapshot, so attach rules, filenames, and toasts stay upstream's.
type asyncPasteState struct {
	enabled bool
	// read is the slow reader, called off the UI goroutine only.
	read     clipboardReader
	inFlight bool
	// applying is set while the snapshot is being replayed through the
	// upstream path, which re-enters the hook; it tells the hook to
	// fall through instead of starting another read.
	applying bool
}

// clipboardSnapshotMsg carries one background clipboard read. bracketed
// is the terminal paste that triggered it, nil for a Ctrl+V keystroke.
type clipboardSnapshotMsg struct {
	image, text []byte
	bracketed   *tea.PasteMsg
}

// SetAsyncClipboardReader makes fn the clipboard paste reads, in place
// of the desktop service's, and routes paste through the background
// path, so the UI paints pastingToast while fn runs. Also marks the
// clipboard available: a bridged reader needs no native init.
func (a *App) SetAsyncClipboardReader(fn clipboardReader) {
	a.SetClipboardAvailable(true)
	a.asyncPaste.enabled = true
	a.asyncPaste.read = fn
}

// pasteAsync is the hook the two paste entry points call first. It
// returns handled=false when paste should take the synchronous upstream
// path: async paste is off, or a snapshot is being replayed. Otherwise
// it starts the background read (or swallows the paste when one is
// already running) and reports handled=true.
func (a *App) pasteAsync(bracketed *tea.PasteMsg) (tea.Cmd, bool) {
	if !a.asyncPaste.enabled || a.asyncPaste.applying {
		return nil, false
	}
	if a.asyncPaste.inFlight {
		return nil, true
	}
	a.asyncPaste.inFlight = true
	a.statusbar.SetToast(pastingToast)
	read := a.asyncPaste.read
	return func() tea.Msg {
		msg := clipboardSnapshotMsg{bracketed: bracketed}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); msg.image = read(core.ClipboardImage) }()
		go func() { defer wg.Done(); msg.text = read(core.ClipboardText) }()
		wg.Wait()
		return msg
	}, true
}

// reducePasteAsync replays a finished background read through the
// upstream paste path.
var reducePasteAsync reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(clipboardSnapshotMsg)
	if !ok {
		return nil, false
	}
	a.asyncPaste.inFlight = false
	a.statusbar.SetToast("")
	if a.mode != ModeInsert {
		return nil, true
	}
	prev := a.desktop
	a.desktop = clipboardSnapshot{DesktopService: prev, image: m.image, text: m.text}
	a.asyncPaste.applying = true
	var cmd tea.Cmd
	if m.bracketed != nil {
		cmd = reducePaste(a, *m.bracketed)
	} else {
		cmd = a.smartPaste()
	}
	a.asyncPaste.applying = false
	a.desktop = prev
	return cmd, true
}

// clipboardSnapshot is the desktop service with its clipboard replaced
// by one finished background read.
type clipboardSnapshot struct {
	core.DesktopService
	image, text []byte
}

func (c clipboardSnapshot) ReadClipboard(f core.ClipboardFormat) []byte {
	switch f {
	case core.ClipboardImage:
		return c.image
	case core.ClipboardText:
		return c.text
	}
	return nil
}
