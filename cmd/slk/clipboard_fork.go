package main

import (
	"log"
	"os"

	"github.com/gammons/slk/internal/ui"
)

// wireRemoteClipboard points app's Ctrl+V paste at the host clipboard
// bridge named by SLK_CLIPBOARD_ADDR ("host:port"), when set. Inside
// the container clipboard.Init() has already failed and main.go logged
// its "clipboard init failed" warning; this supersedes it, so paste is
// enabled after all.
func wireRemoteClipboard(app *ui.App) {
	addr := os.Getenv("SLK_CLIPBOARD_ADDR")
	if addr == "" {
		return
	}
	app.SetAsyncClipboardReader(ui.RemoteClipboardReader(addr))
	log.Printf("Clipboard served by the host bridge at %s; Ctrl+V paste enabled", addr)
}
