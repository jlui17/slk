package image

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

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
	// atomics: the test-only goroutine fallback runs scan on another
	// goroutine that can outlive a timeout return (same hazard
	// sawReject guards against in probeKittyTransmit).
	var gotW, gotH atomic.Int32
	scan := func(buf []byte) (matched, ok bool) {
		matched, cw, ch := scanForCellSize(buf)
		if !matched {
			return false, false
		}
		gotW.Store(int32(cw))
		gotH.Store(int32(ch))
		return true, cw > 0 && ch > 0
	}

	if f, isFile := r.(*os.File); isFile {
		replied, collected, reason := pollProbe(int(f.Fd()), timeout, scan)
		debuglog.ImgRender("cellpx probe: ok=%v reason=%s elapsed_ms=%d cell=%dx%d reply=%q",
			replied, reason, time.Since(start).Milliseconds(), gotW.Load(), gotH.Load(), collected)
		return int(gotW.Load()), int(gotH.Load()), replied
	}

	// Test fallback for non-*os.File readers.
	replied := probeViaGoroutineScan(r, timeout, scan)
	return int(gotW.Load()), int(gotH.Load()), replied
}

// ProbeTerminalVersion sends the XTVERSION query (CSI > 0 q) and
// returns the terminal's name/version reply (DCS > | text ST) verbatim,
// or "" when nothing came back before timeout. The upgrade path uses
// it to identify the terminal behind an identity-stripping mux: a
// transmit ack alone doesn't prove unicode-placeholder support, and
// the terminals known to lack it (see LacksUnicodePlaceholders) do
// answer XTVERSION. Inputs match ProbeKittyGraphics; must run before
// bubbletea takes over stdin.
func ProbeTerminalVersion(w io.Writer, r io.Reader, timeout time.Duration) string {
	if _, err := io.WriteString(w, "\x1b[>0q"); err != nil {
		return ""
	}

	start := time.Now()
	// name is read only after the probe reports a match: the poll path
	// runs scan on this goroutine, and the goroutine fallback's channel
	// receive orders the closure's write before the read.
	var name string
	scan := func(buf []byte) (matched, ok bool) {
		matched, text := scanForXTVersion(buf)
		if !matched {
			return false, false
		}
		name = text
		return true, true
	}

	if f, isFile := r.(*os.File); isFile {
		replied, collected, reason := pollProbe(int(f.Fd()), timeout, scan)
		debuglog.ImgRender("xtversion probe: ok=%v reason=%s elapsed_ms=%d reply=%q",
			replied, reason, time.Since(start).Milliseconds(), collected)
		if !replied {
			return ""
		}
		return name
	}

	// Test fallback for non-*os.File readers.
	if !probeViaGoroutineScan(r, timeout, scan) {
		return ""
	}
	return name
}

// scanForXTVersion returns (matched, text). matched is true once a
// complete XTVERSION report (DCS > | text ST) is present in buf; text
// is the name/version string between the markers.
func scanForXTVersion(buf []byte) (matched bool, text string) {
	i := bytes.Index(buf, []byte("\x1bP>|"))
	if i < 0 {
		return false, ""
	}
	tail := buf[i+4:]
	j := bytes.Index(tail, []byte("\x1b\\"))
	if j < 0 {
		return false, ""
	}
	return true, string(tail[:j])
}

// scanForCellSize returns (matched, pxW, pxH). matched is true once a
// complete XTWINOPS cell-size report (CSI 6 ; height ; width t) is
// present in buf; pxW/pxH are zero when the report's values are (the
// terminal's way of saying it doesn't know). The anchor "\x1b[6;" is
// not unique to the report — a modified-PageDown keystroke encodes as
// \x1b[6;2~ and can land in the raw-mode probe window — so an anchor
// whose body isn't digits-and-semicolons ending in 't' is skipped and
// the scan continues, rather than poisoning a genuine reply sitting
// behind the noise. A late DA1 reply from the sixel probe cannot
// anchor at all — it starts "\x1b[?".
func scanForCellSize(buf []byte) (matched bool, pxW, pxH int) {
	for {
		i := bytes.Index(buf, []byte("\x1b[6;"))
		if i < 0 {
			return false, 0, 0
		}
		tail := buf[i+4:]
		j := 0
		for j < len(tail) && (tail[j] == ';' || (tail[j] >= '0' && tail[j] <= '9')) {
			j++
		}
		if j == len(tail) {
			// Body still valid but unterminated: the reply may be
			// arriving byte by byte, keep waiting.
			return false, 0, 0
		}
		if tail[j] != 't' {
			buf = tail[j:] // not the report (e.g. \x1b[6;2~): skip anchor
			continue
		}
		parts := bytes.Split(tail[:j], []byte(";"))
		if len(parts) != 2 {
			buf = tail[j+1:]
			continue
		}
		h, errH := strconv.Atoi(string(parts[0]))
		w, errW := strconv.Atoi(string(parts[1]))
		if errH != nil || errW != nil {
			buf = tail[j+1:]
			continue
		}
		if w <= 0 || h <= 0 {
			return true, 0, 0
		}
		return true, w, h
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
