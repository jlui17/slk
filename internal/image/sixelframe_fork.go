package image

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// appendFrameCells follows the erase with the flushed frame's own cells
// for every placement it cleared. ECH is what removes sixel pixels, but
// it blanks the cells too, and Bubble Tea's diff rewrites only the cells
// whose text changed since its previous frame: a blanked cell the diff
// skips kept the erase's background, as the run of untinted cells on a
// selected row that scrolled to where an image sat. Rewriting the cells
// from the frame leaves the screen as Bubble Tea believes it is.
func appendFrameCells(erase []byte, gone []SixelPlacement, screen string) []byte {
	if len(gone) == 0 || screen == "" {
		return erase
	}
	lines := strings.Split(screen, "\n")
	b := bytes.NewBuffer(erase)
	b.WriteString("\x1b7")
	for _, pl := range gone {
		for r := pl.Row - 1; r < pl.Row-1+pl.Rows && r < len(lines); r++ {
			cells, start := lineCells(lines[r], pl.Col-1, pl.Col-1+pl.Cols)
			if start < 0 {
				continue
			}
			// Each row starts from a reset and closes any hyperlink, so
			// one row's style never leaks into the next or into the diff.
			fmt.Fprintf(b, "\x1b[0m\x1b[%d;%dH%s\x1b[0m\x1b]8;;\x1b\\", r+1, start+1, cells)
		}
	}
	b.WriteString("\x1b8")
	return b.Bytes()
}

// lineCells returns the part of line that draws columns [left, right),
// led by every escape sequence before it so it draws in the style the
// line has there, and the 0-based column it starts at: left, or one
// before when a wide character straddles left. start is -1 when the line
// ends before left.
func lineCells(line string, left, right int) (cells string, start int) {
	var b strings.Builder
	start = -1
	col := 0
	var state byte
	for len(line) > 0 && col < right {
		seq, width, n, next := ansi.DecodeSequence(line, state, nil)
		state, line = next, line[n:]
		if width > 0 && col+width <= left {
			col += width
			continue
		}
		if width > 0 && start < 0 {
			start = col
		}
		b.WriteString(seq)
		col += width
	}
	return b.String(), start
}
