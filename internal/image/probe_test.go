package image

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProbeKittyGraphics_TimeoutFails(t *testing.T) {
	t.Setenv("TMUX", "")
	r := blockingReader{}
	var w bytes.Buffer
	ok, rejected := ProbeKittyGraphics(&w, r, 50*time.Millisecond)
	if ok || rejected {
		t.Errorf("expected (false, false) on timeout, got (%v, %v)", ok, rejected)
	}
	if !strings.Contains(w.String(), "\x1b_G") {
		t.Errorf("expected \\e_G in probe output, got %q", w.String())
	}
}

func TestProbeKittyGraphics_WrapsProbeInTmux(t *testing.T) {
	t.Setenv("TMUX", "/tmp/tmux")
	r := blockingReader{}
	var w bytes.Buffer
	ok, _ := ProbeKittyGraphics(&w, r, 50*time.Millisecond)
	if ok {
		t.Error("expected probe to fail on timeout")
	}
	if !strings.HasPrefix(w.String(), "\x1bPtmux;\x1b\x1b_G") {
		t.Errorf("expected tmux-wrapped kitty probe, got %q", w.String())
	}
}

// TestProbeKittyGraphics_NoStdinTheftAfterTimeout is the regression
// guard for issue #50. The historical implementation leaked a goroutine
// that kept reading from r forever after the probe timed out; that
// goroutine then stole bytes from whatever reader the host installed
// next (bubbletea), making slk unresponsive in zellij / tmux without
// allow-passthrough.
//
// We use os.Pipe to get a real pollable *os.File pair. We expect that:
//  1. ProbeKittyGraphics returns false on timeout.
//  2. A byte written to the pipe AFTER the probe returns is delivered
//     intact to a subsequent reader (i.e. no leaked goroutine
//     intercepts it).
func TestProbeKittyGraphics_NoStdinTheftAfterTimeout(t *testing.T) {
	t.Setenv("TMUX", "")
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	var wbuf bytes.Buffer
	ok, _ := ProbeKittyGraphics(&wbuf, pr, 50*time.Millisecond)
	if ok {
		t.Fatal("expected probe to fail on timeout (pipe never replies)")
	}

	// Write a byte AFTER the probe has timed out. If a leaked
	// goroutine is still reading from pr, it will consume this byte
	// and the Read below will block until the test's deadline.
	if _, err := pw.Write([]byte{'X'}); err != nil {
		t.Fatalf("pipe write: %v", err)
	}

	// Bound the read so the test fails fast on regression instead of
	// hanging.
	if err := pr.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1)
	n, err := pr.Read(buf)
	if err != nil || n != 1 || buf[0] != 'X' {
		t.Fatalf("expected to read 'X' from pipe after probe; got n=%d err=%v buf=%q (probe goroutine likely leaked and stole the byte)",
			n, err, buf[:n])
	}
}

func TestScanForOK_MatchesProbeID(t *testing.T) {
	cases := []struct {
		name        string
		buf         string
		wantID      int
		wantMatched bool
		wantOK      bool
	}{
		{"ok for our id", "\x1b_Gi=9999;OK\x1b\\", 9999, true, true},
		{"error for our id", "\x1b_Gi=9999;EINVAL: unsupported format\x1b\\", 9999, true, false},
		// A slow terminal's reply to an earlier probe (different id)
		// must be skipped, not consumed as this probe's answer.
		{"stale ok skipped", "\x1b_Gi=9998;OK\x1b\\", 9999, false, false},
		{"stale ok then our error", "\x1b_Gi=9998;OK\x1b\\\x1b_Gi=9999;EBADPNG\x1b\\", 9999, true, false},
		{"stale error then our ok", "\x1b_Gi=9999;EINVAL\x1b\\\x1b_Gi=9998;OK\x1b\\", 9998, true, true},
		// i=999 must not match i=9998 as a prefix.
		{"prefix id no match", "\x1b_Gi=9998;OK\x1b\\", 999, false, false},
		// A reply with no id echo is judged on its own ;OK.
		{"no id echoed", "\x1b_G;OK\x1b\\", 9999, true, true},
		{"incomplete", "\x1b_Gi=9999;OK", 9999, false, false},
		{"empty", "", 9999, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, ok := scanForOK([]byte(tc.buf), tc.wantID)
			if matched != tc.wantMatched || ok != tc.wantOK {
				t.Errorf("scanForOK(%q, %d) = (%v, %v), want (%v, %v)",
					tc.buf, tc.wantID, matched, ok, tc.wantMatched, tc.wantOK)
			}
		})
	}
}

func TestProbeKittyGraphics_IgnoresStaleReplyForOtherID(t *testing.T) {
	t.Setenv("TMUX", "")
	var w bytes.Buffer
	// Only a stale RGBA-probe OK is buffered; the PNG probe must not
	// claim it, so it times out instead of succeeding.
	ok, rejected := ProbeKittyGraphics(&w, strings.NewReader("\x1b_Gi=9998;OK\x1b\\"), 100*time.Millisecond)
	if ok || rejected {
		t.Errorf("got (ok=%v, rejected=%v), want (false, false)", ok, rejected)
	}
}

func TestProbeKittyGraphics_RejectedOnErrorReply(t *testing.T) {
	t.Setenv("TMUX", "")
	var w bytes.Buffer
	// herdr's embedded libghostty-vt without a PNG decoder answers
	// exactly this.
	r := strings.NewReader("\x1b_Gi=9999;EINVAL: unsupported format\x1b\\")
	ok, rejected := ProbeKittyGraphics(&w, r, time.Second)
	if ok || !rejected {
		t.Errorf("got (ok=%v, rejected=%v), want (false, true)", ok, rejected)
	}
}

func TestProbeKittyRGBA_SendsRawTransmit(t *testing.T) {
	t.Setenv("TMUX", "")
	var w bytes.Buffer
	ok, rejected := ProbeKittyRGBA(&w, strings.NewReader("\x1b_Gi=9998;OK\x1b\\"), time.Second)
	if !ok || rejected {
		t.Errorf("got (ok=%v, rejected=%v), want (true, false)", ok, rejected)
	}
	if !strings.Contains(w.String(), "f=32,s=1,v=1") {
		t.Errorf("expected raw RGBA probe header, got %q", w.String())
	}
}

func TestScanForSixelDA(t *testing.T) {
	cases := []struct {
		name        string
		buf         string
		wantMatched bool
		wantOK      bool
	}{
		{"xterm vt340", "\x1b[?63;1;2;4;6;9;15;22c", true, true},
		{"foot", "\x1b[?62;4;22c", true, true},
		{"sixel only", "\x1b[?4c", true, true},
		{"no sixel", "\x1b[?62;1;6;22c", true, false},
		// Attributes are semicolon-separated values, not substrings:
		// 14 / 24 / 41 are unrelated capabilities.
		{"substring not attribute", "\x1b[?14;24;41c", true, false},
		{"incomplete: no terminator", "\x1b[?62;4;22", false, false},
		{"incomplete: no csi", "62;4;22c", false, false},
		{"empty", "", false, false},
		// Terminals emit the reply amid whatever else is in flight.
		{"leading noise", "junk\x1b[?62;4;22c", true, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, ok := scanForSixelDA([]byte(tc.buf))
			if matched != tc.wantMatched || ok != tc.wantOK {
				t.Errorf("scanForSixelDA(%q) = (%v, %v), want (%v, %v)",
					tc.buf, matched, ok, tc.wantMatched, tc.wantOK)
			}
		})
	}
}

func TestProbeSixel_SendsDA1AndFailsOnTimeout(t *testing.T) {
	var w bytes.Buffer
	if ok := ProbeSixel(&w, blockingReader{}, 50*time.Millisecond); ok {
		t.Error("expected probe to fail on timeout")
	}
	if w.String() != "\x1b[c" {
		t.Errorf("expected DA1 query, got %q", w.String())
	}
}

func TestProbeSixel_ReadsReply(t *testing.T) {
	cases := []struct {
		reply string
		want  bool
	}{
		{"\x1b[?63;1;2;4;6;9;15;22c", true},
		{"\x1b[?62;1;6;22c", false},
	}
	for _, tc := range cases {
		t.Run(tc.reply, func(t *testing.T) {
			var w bytes.Buffer
			r := strings.NewReader(tc.reply)
			if got := ProbeSixel(&w, r, time.Second); got != tc.want {
				t.Errorf("ProbeSixel with reply %q = %v, want %v", tc.reply, got, tc.want)
			}
		})
	}
}

// The poll-based production path (real *os.File) must reach the same
// answer as the goroutine fallback, and must not leave a reader behind
// that steals bytes from bubbletea afterwards (the issue #50 hazard).
func TestProbeSixel_PollPathOnRealPipe(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	if _, err := pw.Write([]byte("\x1b[?62;4;22c")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	var w bytes.Buffer
	if ok := ProbeSixel(&w, pr, time.Second); !ok {
		t.Fatal("expected sixel-capable DA1 reply to be recognized")
	}

	if _, err := pw.Write([]byte{'X'}); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	if err := pr.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1)
	n, err := pr.Read(buf)
	if err != nil || n != 1 || buf[0] != 'X' {
		t.Fatalf("expected to read 'X' from pipe after probe; got n=%d err=%v buf=%q",
			n, err, buf[:n])
	}
}

func TestScanForCellSize(t *testing.T) {
	cases := []struct {
		name        string
		buf         string
		wantMatched bool
		wantW       int
		wantH       int
	}{
		{"ghostty retina", "\x1b[6;38;18t", true, 18, 38},
		{"zero values", "\x1b[6;0;0t", true, 0, 0},
		// Wrong shape at the anchor is skipped, not judged: keep
		// waiting for a real report.
		{"malformed: missing field", "\x1b[6;38t", false, 0, 0},
		{"malformed: non-numeric", "\x1b[6;a;bt", false, 0, 0},
		{"incomplete: no terminator", "\x1b[6;38;18", false, 0, 0},
		{"empty", "", false, 0, 0},
		// A late DA1 reply from the sixel probe starts \x1b[? and must
		// not anchor.
		{"da1 reply ignored", "\x1b[?62;4;22c", false, 0, 0},
		{"leading noise", "junk\x1b[6;38;18t", true, 18, 38},
		// A modified-PageDown keystroke (\x1b[6;2~) contains the anchor;
		// it must not poison the genuine report behind it.
		{"keystroke then report", "\x1b[6;2~\x1b[6;38;18t", true, 18, 38},
		{"keystroke only", "\x1b[6;5~", false, 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, w, h := scanForCellSize([]byte(tc.buf))
			if matched != tc.wantMatched || w != tc.wantW || h != tc.wantH {
				t.Errorf("scanForCellSize(%q) = (%v, %d, %d), want (%v, %d, %d)",
					tc.buf, matched, w, h, tc.wantMatched, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestProbeCellPixels_SendsQueryAndFailsOnTimeout(t *testing.T) {
	var w bytes.Buffer
	if _, _, ok := ProbeCellPixels(&w, blockingReader{}, 50*time.Millisecond); ok {
		t.Error("expected probe to fail on timeout")
	}
	if w.String() != "\x1b[16t" {
		t.Errorf("expected XTWINOPS 16t query, got %q", w.String())
	}
}

func TestProbeCellPixels_ReadsReply(t *testing.T) {
	var w bytes.Buffer
	pxW, pxH, ok := ProbeCellPixels(&w, strings.NewReader("\x1b[6;38;18t"), time.Second)
	if !ok || pxW != 18 || pxH != 38 {
		t.Errorf("ProbeCellPixels = (%d, %d, %v), want (18, 38, true)", pxW, pxH, ok)
	}
}

func TestProbeCellPixels_PollPathOnRealPipe(t *testing.T) {
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer pr.Close()
	defer pw.Close()

	if _, err := pw.Write([]byte("\x1b[6;38;18t")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	var w bytes.Buffer
	pxW, pxH, ok := ProbeCellPixels(&w, pr, time.Second)
	if !ok || pxW != 18 || pxH != 38 {
		t.Fatalf("ProbeCellPixels = (%d, %d, %v), want (18, 38, true)", pxW, pxH, ok)
	}
}

type blockingReader struct{}

func (blockingReader) Read(p []byte) (int, error) {
	time.Sleep(time.Hour)
	return 0, nil
}
