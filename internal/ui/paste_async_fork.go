package ui

import (
	"sync"

	tea "charm.land/bubbletea/v2"
	"golang.design/x/clipboard"
)

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
	enabled  bool
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

// SetAsyncClipboardReader installs fn as the clipboard reader and routes
// paste through the background path, so the UI paints pastingToast
// while fn runs. Also marks the clipboard available: a bridged reader
// needs no native init.
func (a *App) SetAsyncClipboardReader(fn clipboardReader) {
	a.SetClipboardReader(fn)
	a.SetClipboardAvailable(true)
	a.asyncPaste.enabled = true
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
	read := a.clipboardRead
	return func() tea.Msg {
		msg := clipboardSnapshotMsg{bracketed: bracketed}
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); msg.image = read(clipboard.FmtImage) }()
		go func() { defer wg.Done(); msg.text = read(clipboard.FmtText) }()
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
	prev := a.clipboardRead
	a.clipboardRead = func(format clipboard.Format) []byte {
		switch format {
		case clipboard.FmtImage:
			return m.image
		case clipboard.FmtText:
			return m.text
		}
		return nil
	}
	a.asyncPaste.applying = true
	var cmd tea.Cmd
	if m.bracketed != nil {
		cmd = reducePaste(a, *m.bracketed)
	} else {
		cmd = a.smartPaste()
	}
	a.asyncPaste.applying = false
	a.clipboardRead = prev
	return cmd, true
}
