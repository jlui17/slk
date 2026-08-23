package image

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

// ProbeKittyGraphics sends a tiny PNG upload with response requested
// and waits up to timeout for the OK reply. ok is true if the terminal
// acknowledges. rejected is true when a complete reply arrived that
// was NOT an OK — the terminal speaks the graphics protocol but
// refused this transmit — as opposed to no reply at all (a mux
// swallowed the escape, or no kitty support whatsoever). Callers use
// rejected to decide whether trying another pixel format is worth a
// second probe: libghostty-vt embedders without a wired PNG decoder
// (herdr) answer "EINVAL: unsupported format" here but accept raw
// RGBA (see ProbeKittyRGBA). Used at startup to downgrade ProtoKitty
// when the terminal claims kitty support but doesn't actually deliver
// (e.g., iTerm2's limited kitty implementation, or zellij / tmux with
// allow-passthrough=off swallowing the probe escape).
//
// Inputs:
//
//	w:       terminal writer (typically os.Stdout)
//	r:       terminal reader (typically os.Stdin in raw mode)
//	timeout: how long to wait for the reply
//
// Implementation note (issue #50): the production path uses pollProbe
// (poll(2) + read(2), see probe_unix.go) so the function is fully
// synchronous and spawns no goroutine. Earlier implementations spawned
// a goroutine that kept reading from r forever after the select-on-
// timeout returned. That leaked goroutine then raced bubbletea's input
// loop for every byte the user typed, discarding ~95% of keystrokes
// (most aren't 0x1b) and making slk unresponsive whenever the probe
// timed out -- which is exactly when the user is in zellij or in tmux
// with allow-passthrough=off, because the multiplexer swallows the
// probe escape and no reply ever arrives.
//
// The poll-based path needs r to be an *os.File (any Go file with a
// real fd works: os.Stdin, os.Pipe). For non-*os.File readers
// (blockingReader in tests), this falls back to the goroutine-based
// probe; that path may leak a goroutine on timeout but tests exit
// immediately so it doesn't matter.
func ProbeKittyGraphics(w io.Writer, r io.Reader, timeout time.Duration) (ok, rejected bool) {
	// Minimal valid 1x1 PNG.
	const tinyPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+P+/HgAFhAJ/wlseKgAAAABJRU5ErkJggg=="
	header := fmt.Sprintf("a=T,f=100,t=d,i=%d,q=0", kittyProbeIDPNG)
	return probeKittyTransmit(w, r, timeout, "png probe", kittyProbeIDPNG, header, tinyPNG)
}

// ProbeSixel sends a Primary Device Attributes query (CSI c) and reports
// whether the reply advertises sixel graphics — attribute 4 in the
// DA1 response, e.g. `\e[?62;1;4;6;22c` from xterm -ti vt340.
//
// This is the same runtime check img2sixel / chafa use, and it is the
// only reliable one: the set of sixel-capable terminals is open-ended
// (foot, xterm, mlterm, WezTerm, contour, DomTerm, toyterm, …) and
// several of them share a generic TERM value, so a TERM allowlist
// silently downgrades them to the half-block mosaic (issue #116).
//
// Every terminal answers DA1, including ones with no sixel support, so
// unlike the kitty probe a timeout here means "no reply at all" (a
// multiplexer swallowed it, or stdin isn't really the terminal) rather
// than "no sixel". Both outcomes are reported as false.
//
// Inputs match ProbeKittyGraphics: w is the terminal writer, r the
// terminal reader in raw mode, timeout the reply deadline. Must run
// before bubbletea takes over stdin.
func ProbeSixel(w io.Writer, r io.Reader, timeout time.Duration) bool {
	if _, err := io.WriteString(w, "\x1b[c"); err != nil {
		return false
	}

	start := time.Now()

	if f, ok := r.(*os.File); ok {
		ok, collected, reason := pollProbe(int(f.Fd()), timeout, scanForSixelDA)
		debuglog.ImgRender("sixel probe: ok=%v reason=%s elapsed_ms=%d reply=%q",
			ok, reason, time.Since(start).Milliseconds(), collected)
		return ok
	}

	// Test fallback for non-*os.File readers.
	return probeViaGoroutineScan(r, timeout, scanForSixelDA)
}

// probeViaGoroutineScan is the generic goroutine-based probe used for
// non-*os.File readers (tests only). It reads byte by byte, re-running
// scan over everything collected so far, and stops once scan reports a
// match.
func probeViaGoroutineScan(r io.Reader, timeout time.Duration, scan func([]byte) (bool, bool)) bool {
	ch := make(chan bool, 1)
	go func() {
		br := bufio.NewReader(r)
		var collected []byte
		for {
			b, err := br.ReadByte()
			if err != nil {
				ch <- false
				return
			}
			collected = append(collected, b)
			if matched, ok := scan(collected); matched {
				ch <- ok
				return
			}
		}
	}()
	select {
	case ok := <-ch:
		return ok
	case <-time.After(timeout):
		return false
	}
}

// scanForSixelDA returns (matched, ok). matched is true once a complete
// DA1 reply (CSI ? … c) is present in buf; ok is true when that reply
// lists attribute 4 (sixel graphics). Attributes are semicolon-
// separated and must match exactly — "14" or "24" are unrelated
// capabilities, so a substring test would be wrong.
func scanForSixelDA(buf []byte) (matched, ok bool) {
	i := bytes.Index(buf, []byte("\x1b[?"))
	if i < 0 {
		return false, false
	}
	tail := buf[i+3:] // skip past \x1b[?
	j := bytes.IndexByte(tail, 'c')
	if j < 0 {
		return false, false
	}
	for _, attr := range bytes.Split(tail[:j], []byte(";")) {
		if bytes.Equal(attr, []byte("4")) {
			return true, true
		}
	}
	return true, false
}

// scanForOK returns (matched, ok). matched is true once a complete
// kitty graphics response (\x1b_G ... \x1b\\) addressed to wantID is
// present in buf; ok is true when that reply's payload contains ";OK".
// A complete reply echoing a DIFFERENT id is skipped, not judged: it
// is a stale answer to an earlier probe still sitting in the input
// buffer (a slow terminal can reply after the earlier probe's timeout
// already returned), and consuming it as this probe's answer is how a
// probe sequence reaches the wrong protocol verdict. A reply echoing
// no id at all is judged on its own ;OK — there is nothing to
// disambiguate on, and a minimal implementation may omit the echo.
func scanForOK(buf []byte, wantID int) (matched, ok bool) {
	want := []byte(fmt.Sprintf("i=%d", wantID))
	for {
		i := bytes.Index(buf, []byte("\x1b_G"))
		if i < 0 {
			return false, false
		}
		tail := buf[i+3:] // skip past \x1b_G
		j := bytes.Index(tail, []byte("\x1b\\"))
		if j < 0 {
			return false, false
		}
		reply := tail[:j]
		if kittyReplyMatchesID(reply, want) {
			return true, bytes.Contains(reply, []byte(";OK"))
		}
		buf = tail[j+2:] // stale reply for another id: skip past it
	}
}
