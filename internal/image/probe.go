package image

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
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

// ProbeKittyRGBA is ProbeKittyGraphics with a raw-RGBA payload (f=32,
// one opaque white pixel) instead of PNG. Raw pixel formats are the
// part of the kitty graphics protocol every implementation decodes
// natively — libghostty-vt embedders that reject PNG (no decoder
// wired) still accept these — so an OK here means uploads work as
// long as they're encoded raw (see SetKittyUploadRGBA). rejected has
// the same meaning as in ProbeKittyGraphics.
func ProbeKittyRGBA(w io.Writer, r io.Reader, timeout time.Duration) (ok, rejected bool) {
	const rawWhitePixel = "/////w==" // base64 of 4×0xff: one RGBA pixel
	header := fmt.Sprintf("a=T,f=32,s=1,v=1,t=d,i=%d,q=0", kittyProbeIDRGBA)
	return probeKittyTransmit(w, r, timeout, "rgba probe", kittyProbeIDRGBA, header, rawWhitePixel)
}

// Distinct probe image IDs so a late reply to one probe can never be
// misread as the answer to the other.
const (
	kittyProbeIDPNG  = 9999
	kittyProbeIDRGBA = 9998
)

// probeKittyTransmit sends one kitty graphics transmit and classifies
// the terminal's answer: (true, false) on ;OK, (false, true) on a
// complete non-OK reply, (false, false) when nothing came back before
// timeout. tag labels the debug log line.
func probeKittyTransmit(w io.Writer, r io.Reader, timeout time.Duration, tag string, id int, header, payload string) (ok, rejected bool) {
	if err := writeKittySequence(w, fmt.Sprintf("\x1b_G%s;%s\x1b\\", header, payload)); err != nil {
		return false, false
	}

	start := time.Now()
	// atomic: the test-only goroutine fallback runs scan on another
	// goroutine that can outlive a timeout return.
	var sawReject atomic.Bool
	scan := func(buf []byte) (bool, bool) {
		matched, scanOK := scanForOK(buf, id)
		if matched && !scanOK {
			sawReject.Store(true)
		}
		return matched, scanOK
	}

	if f, isFile := r.(*os.File); isFile {
		acked, collected, reason := pollProbe(int(f.Fd()), timeout, scan)
		debuglog.ImgRender("%s: ok=%v reason=%s elapsed_ms=%d reply=%q",
			tag, acked, reason, time.Since(start).Milliseconds(), collected)
		return acked, sawReject.Load()
	}

	// Test fallback for non-*os.File readers.
	return probeViaGoroutineScan(r, timeout, scan), sawReject.Load()
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

// ProbeCellPixels asks the terminal for its cell size in pixels via
// the XTWINOPS query CSI 16t; the reply is CSI 6 ; height ; width t.
//
// This exists because TIOCGWINSZ pixel fields don't survive every pty:
// docker's -it pty and some multiplexers propagate rows/cols but leave
// ws_xpixel/ws_ypixel at zero, so CellPixels falls back to 8x16 and
// the pixel-addressed renderers encode a raster far smaller than a
// hidpi cell box — the terminal stretches it back up and images render
// soft. The escape query rides through those layers like the protocol
// probes do.
//
// Inputs match ProbeKittyGraphics: w is the terminal writer, r the
// terminal reader in raw mode, timeout the reply deadline. Must run
// before bubbletea takes over stdin. ok is false on timeout (the
// terminal doesn't implement 16t, or a mux swallowed it) and on a
// malformed or zero-valued reply.
func ProbeCellPixels(w io.Writer, r io.Reader, timeout time.Duration) (pxW, pxH int, ok bool) {
	if _, err := io.WriteString(w, "\x1b[16t"); err != nil {
		return 0, 0, false
	}

	start := time.Now()
	var gotW, gotH int
	scan := func(buf []byte) (matched, ok bool) {
		matched, cw, ch := scanForCellSize(buf)
		if !matched {
			return false, false
		}
		gotW, gotH = cw, ch
		return true, cw > 0 && ch > 0
	}

	if f, isFile := r.(*os.File); isFile {
		replied, collected, reason := pollProbe(int(f.Fd()), timeout, scan)
		debuglog.ImgRender("cellpx probe: ok=%v reason=%s elapsed_ms=%d cell=%dx%d reply=%q",
			replied, reason, time.Since(start).Milliseconds(), gotW, gotH, collected)
		return gotW, gotH, replied
	}

	// Test fallback for non-*os.File readers.
	return gotW, gotH, probeViaGoroutineScan(r, timeout, scan)
}

// scanForCellSize returns (matched, pxW, pxH). matched is true once a
// complete XTWINOPS cell-size report (CSI 6 ; height ; width t) is
// present in buf; pxW/pxH are zero when the reply is malformed. The
// anchor "\x1b[6;" cannot collide with a late DA1 reply from the sixel
// probe — that one starts "\x1b[?".
func scanForCellSize(buf []byte) (matched bool, pxW, pxH int) {
	i := bytes.Index(buf, []byte("\x1b[6;"))
	if i < 0 {
		return false, 0, 0
	}
	tail := buf[i+4:]
	j := bytes.IndexByte(tail, 't')
	if j < 0 {
		return false, 0, 0
	}
	parts := bytes.Split(tail[:j], []byte(";"))
	if len(parts) != 2 {
		return true, 0, 0
	}
	h, errH := strconv.Atoi(string(parts[0]))
	w, errW := strconv.Atoi(string(parts[1]))
	if errH != nil || errW != nil || w <= 0 || h <= 0 {
		return true, 0, 0
	}
	return true, w, h
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

// kittyReplyMatchesID reports whether a kitty graphics reply's
// comma-separated key block (everything before the first ';') carries
// exactly the wanted i=<id> key. want must be the full "i=<id>" bytes
// so "i=999" never matches "i=9998". A reply with no i= key matches
// unconditionally — see scanForOK.
func kittyReplyMatchesID(reply, want []byte) bool {
	keys := reply
	if end := bytes.IndexByte(reply, ';'); end >= 0 {
		keys = reply[:end]
	}
	if !bytes.Contains(keys, []byte("i=")) {
		return true
	}
	for _, kv := range bytes.Split(keys, []byte(",")) {
		if bytes.Equal(kv, want) {
			return true
		}
	}
	return false
}
