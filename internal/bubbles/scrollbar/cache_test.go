package scrollbar

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

// cacheRows is the row mix of TestOverlayMatchesOracle for one width:
// rows Overlay cuts, keeps whole and pads, with SGR, OSC 8 links, wide
// graphemes (some straddling the gutter column) and combining marks.
func cacheRows(width int) []string {
	return []string{
		"",
		"short",
		strings.Repeat("x", max(width-1, 0)),
		strings.Repeat("y", width),
		strings.Repeat("y", width+7),
		"\x1b[31mred\x1b[0m",
		"\x1b[1;38;2;1;2;3m" + strings.Repeat("z", max(width-1, 0)) + "\x1b[0m",
		"\x1b[32m" + strings.Repeat("w", width+3) + "\x1b[0m",
		"\u4f60\u597d",
		"\x1b]8;;https://example.com\x1b\\link\x1b]8;;\x1b\\",
		"\x1b]8;;https://example.com\x07" + strings.Repeat("l", width+2) + "\x1b]8;;\x07",
		"\U0001F600 smile \U0001F44D\U0001F3FD",
		strings.Repeat("\u4f60", width/2+1),
		strings.Repeat("a", max(width-2, 0)) + "\u4f60",
		strings.Repeat("a", max(width-3, 0)) + "\u4f60\u597d",
		"e\u0301 \U0001F468\u200d\U0001F469\u200d\U0001F467",
	}
}

// TestCacheOverlayMatchesOracle draws a scrolling window over the same
// content through one Cache and checks every frame against the oracle:
// cold, warm, after a line changes under its cached prefix, after the
// background changes and after the width changes.
func TestCacheOverlayMatchesOracle(t *testing.T) {
	bgs := []color.Color{
		lipgloss.Color("#123456"),
		lipgloss.Color("5"),
		nil,
		lipgloss.NoColor{},
	}
	const height = 7
	var c Cache
	for _, width := range []int{1, 2, 3, 5, 80, 81, 200, 80} {
		for name, bgc := range bgs {
			content := append(cacheRows(width), cacheRows(width)...)
			total := len(content)
			frames := []int{0, 1, 2, 9, total - height, total - height - 1, 3, 3}
			for f, off := range frames {
				if f == len(frames)-1 {
					// A line under a warm entry changes; the entry must not be used.
					content[off+1] = strings.Repeat("q", width+1)
				}
				win := content[off : off+height]
				got := c.Overlay(append([]string(nil), win...), width, total, off, height, bgc, track, thumb)
				want := overlayOracle(append([]string(nil), win...), width, total, off, height, bgc, track, thumb)
				for i := range want {
					if got[i] != want[i] {
						t.Fatalf("bg=%d width=%d frame %d off=%d row %d:\n got  %q\n want %q", name, width, f, off, i, got[i], want[i])
					}
				}
			}
		}
	}
}
