package ui

import (
	"io"
	"log"
	"net/http"
	"time"

	"golang.design/x/clipboard"
)

// RemoteClipboardReader returns a clipboardReader that fetches the
// clipboard from a host-side HTTP bridge at addr ("host:port"). Used
// when slk runs in a container (see tools/run-docker.sh), where
// golang.design/x/clipboard has no X11 display to init against and
// the container can't see the host's clipboard at all; the bridge in
// tools/clipboard-bridge.py runs on the host and serves the macOS
// clipboard. GET /image answers 200 with PNG bytes, GET /text with
// UTF-8 text; either answers 204 when the clipboard holds nothing of
// that kind.
func RemoteClipboardReader(addr string) clipboardReader {
	// osascript on the host answers in ~0.25s; a large image takes
	// longer, so leave headroom without letting Ctrl+V hang the UI.
	client := &http.Client{Timeout: 5 * time.Second}
	return func(format clipboard.Format) []byte {
		switch format {
		case clipboard.FmtImage:
			return bridgeGet(client, addr, "/image")
		case clipboard.FmtText:
			return bridgeGet(client, addr, "/text")
		}
		return nil
	}
}

// bridgeGet fetches http://addr+path and returns the body on 200. It
// returns nil on 204 (nothing of that kind on the clipboard) and,
// with a log line, on any error or other status.
func bridgeGet(client *http.Client, addr, path string) []byte {
	resp, err := client.Get("http://" + addr + path)
	if err != nil {
		log.Printf("[paste] clipboard bridge %s: GET %s failed: %v", addr, path, err)
		return nil
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[paste] clipboard bridge %s: reading %s body: %v", addr, path, err)
			return nil
		}
		return body
	case http.StatusNoContent:
		return nil
	}
	log.Printf("[paste] clipboard bridge %s: GET %s returned %s", addr, path, resp.Status)
	return nil
}
