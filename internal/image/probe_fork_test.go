package image

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

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

func TestScanForXTVersion(t *testing.T) {
	cases := []struct {
		name        string
		buf         string
		wantMatched bool
		wantText    string
	}{
		{"wezterm", "\x1bP>|WezTerm 20240203-110809-5046fc22\x1b\\", true, "WezTerm 20240203-110809-5046fc22"},
		{"ghostty", "\x1bP>|ghostty 1.1.3\x1b\\", true, "ghostty 1.1.3"},
		{"incomplete", "\x1bP>|WezTerm", false, ""},
		{"empty", "", false, ""},
		{"leading noise", "junk\x1bP>|kitty(0.32.2)\x1b\\", true, "kitty(0.32.2)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			matched, text := scanForXTVersion([]byte(tc.buf))
			if matched != tc.wantMatched || text != tc.wantText {
				t.Errorf("scanForXTVersion(%q) = (%v, %q), want (%v, %q)",
					tc.buf, matched, text, tc.wantMatched, tc.wantText)
			}
		})
	}
}

func TestProbeTerminalVersion(t *testing.T) {
	var w bytes.Buffer
	got := ProbeTerminalVersion(&w, strings.NewReader("\x1bP>|WezTerm 2024\x1b\\"), time.Second)
	if got != "WezTerm 2024" {
		t.Errorf("ProbeTerminalVersion = %q, want %q", got, "WezTerm 2024")
	}
	if w.String() != "\x1b[>0q" {
		t.Errorf("expected XTVERSION query, got %q", w.String())
	}
	if got := ProbeTerminalVersion(&bytes.Buffer{}, blockingReader{}, 50*time.Millisecond); got != "" {
		t.Errorf("expected empty name on timeout, got %q", got)
	}
}

func TestLacksUnicodePlaceholders(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"WezTerm 20240203-110809-5046fc22", true},
		{"iTerm2 3.5.10", true},
		{"ghostty 1.1.3", false},
		{"kitty(0.32.2)", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := LacksUnicodePlaceholders(tc.name); got != tc.want {
			t.Errorf("LacksUnicodePlaceholders(%q) = %v, want %v", tc.name, got, tc.want)
		}
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
