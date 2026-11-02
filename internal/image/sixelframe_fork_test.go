package image

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The App reuses a published frame's ID when nothing sixel-relevant
// changed, so the same marked ID can reach FrameOutput again after its
// first flush already pruned it from the store (Take prunes <= id). That
// second flush must be a pure text passthrough: guard equality is what
// allowed the reuse, so the cells under every image are unchanged and
// the painter's on-screen state is still accurate — ok=false, empty
// plan, painter untouched.
func TestFrameOutput_ReusedFrameIDAfterPruneIsNoOp(t *testing.T) {
	var raw bytes.Buffer
	frames := NewSixelFrameStore()
	out := NewFrameOutput(&raw, frames)

	id := frames.Publish(SixelFrame{Placements: []SixelPlacement{pl("img", 2, 3, 1, 1)}})
	if _, err := out.Write(markedFlush("slk", id, "text-1")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	if out.painter.Live() != 1 {
		t.Fatalf("Live = %d after first flush, want 1", out.painter.Live())
	}
	if !strings.Contains(raw.String(), "\x1bPqimg\x1b\\") {
		t.Fatalf("first flush did not paint: %q", raw.String())
	}

	raw.Reset()
	if _, err := out.Write(markedFlush("slk", id, "text-2")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	if got := raw.String(); got != "text-2" {
		t.Fatalf("reused-ID flush must pass text through untouched; got %q", got)
	}
	if out.painter.Live() != 1 {
		t.Fatalf("reused-ID flush mutated painter state; Live = %d, want 1", out.painter.Live())
	}
}

// testTerm is the slice of a terminal that FrameOutput's own bytes need:
// cursor addressing (CUP, DECSC/DECRC), the SGR background, ECH and
// printing. Each cell keeps its text and background, so a test sees what
// a flush leaves on screen in the cells the text diff did not write.
type testTerm struct {
	cells [][]termCell
	cur   termCursor
	saved termCursor
}

type termCell struct{ text, bg string }

type termCursor struct {
	row, col int // 0-based
	bg       string
}

func newTestTerm(w, h int) *testTerm {
	t := &testTerm{cells: make([][]termCell, h)}
	for r := range t.cells {
		t.cells[r] = make([]termCell, w)
	}
	return t
}

// drawFrame writes each line of screen from the start of its row, as a
// full redraw of that frame would.
func (t *testTerm) drawFrame(screen string) {
	for r, line := range strings.Split(screen, "\n") {
		t.write(fmt.Sprintf("\x1b[%d;1H\x1b[0m%s\x1b[0m", r+1, line))
	}
}

func (t *testTerm) write(s string) {
	var state byte
	for len(s) > 0 {
		seq, width, n, next := ansi.DecodeSequence(s, state, nil)
		state, s = next, s[n:]
		switch {
		case width > 0:
			t.print(seq, width)
		case seq == "\x1b7":
			t.saved = t.cur
		case seq == "\x1b8":
			t.cur = t.saved
		case ansi.HasCsiPrefix(seq) && !strings.HasPrefix(seq, "\x1b[?"):
			t.csi(seq[len(seq)-1], strings.Split(seq[2:len(seq)-1], ";"))
		}
	}
}

func (t *testTerm) print(text string, width int) {
	row := t.cells[t.cur.row]
	for c := t.cur.col; c < t.cur.col+width && c < len(row); c++ {
		row[c] = termCell{bg: t.cur.bg}
	}
	if t.cur.col < len(row) {
		row[t.cur.col].text = text
	}
	t.cur.col += width
}

func (t *testTerm) csi(final byte, params []string) {
	num := func(i int) int {
		if i < len(params) {
			if n, err := strconv.Atoi(params[i]); err == nil && n > 0 {
				return n
			}
		}
		return 1
	}
	switch final {
	case 'H':
		t.cur.row, t.cur.col = num(0)-1, num(1)-1
	case 'X':
		row := t.cells[t.cur.row]
		for c := t.cur.col; c < t.cur.col+num(0) && c < len(row); c++ {
			row[c] = termCell{bg: t.cur.bg}
		}
	case 'm':
		for i := 0; i < len(params); i++ {
			switch {
			case params[i] == "" || params[i] == "0" || params[i] == "49":
				t.cur.bg = ""
			case params[i] == "48" && i+4 < len(params) && params[i+1] == "2":
				t.cur.bg = strings.Join(params[i:i+5], ";")
				i += 4
			case params[i] == "38" && i+4 < len(params) && params[i+1] == "2":
				i += 4
			}
		}
	}
}

// When an image goes away, its erase must leave the vacated cells as the
// flushed frame draws them. Bubble Tea's diff skips every cell whose text
// did not change since the previous frame, so a cell the erase blanks to
// the theme background and the diff skips keeps the theme background:
// the selected message's tinted header row, scrolled up to where an image
// sat, showed a run of untinted cells. The second image's left edge cuts
// a wide character, which must come back whole.
func TestFrameOutput_EraseLeavesVacatedCellsAsTheFrameDrawsThem(t *testing.T) {
	const themeBg, tint = "\x1b[48;2;9;9;9m", "\x1b[48;2;1;2;3m"
	screen := strings.Join([]string{
		themeBg + "            ",
		themeBg + "ab" + tint + "        " + themeBg + "cd",
		themeBg + "a界" + tint + "       " + themeBg + "cd",
	}, "\n")
	want := newTestTerm(12, 3)
	want.drawFrame(screen)

	var raw bytes.Buffer
	frames := NewSixelFrameStore()
	out := NewFrameOutput(&raw, frames)
	term := newTestTerm(12, 3)
	term.drawFrame(screen)

	images := []SixelPlacement{pl("one", 2, 3, 1, 8), pl("two", 3, 3, 1, 8)}
	id := frames.Publish(SixelFrame{Placements: images, EraseSGR: themeBg, Screen: screen})
	if _, err := out.Write(markedFlush("slk", id, "")); err != nil {
		t.Fatalf("Write 1: %v", err)
	}
	term.write(raw.String())
	raw.Reset()

	// The images leave and the text under them is unchanged, so the
	// text diff for this frame is empty.
	id = frames.Publish(SixelFrame{EraseSGR: themeBg, Screen: screen})
	if _, err := out.Write(markedFlush("slk", id, "")); err != nil {
		t.Fatalf("Write 2: %v", err)
	}
	term.write(raw.String())

	for r := range want.cells {
		for c := range want.cells[r] {
			if got, w := term.cells[r][c], want.cells[r][c]; got != w {
				t.Errorf("row %d col %d = %+v, want %+v as the frame draws it", r+1, c+1, got, w)
			}
		}
	}
}
