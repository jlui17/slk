// Package cellwidth fits unstyled text to a number of terminal cells as
// lipgloss.Width counts them, the measure boxes and columns are padded by.
package cellwidth

import (
	"charm.land/lipgloss/v2"
	"github.com/rivo/uniseg"
)

// Cut ends s in … when it is wider than width cells, as lipgloss.Width
// counts them: the rows and columns are padded by that measure, so a cut
// by any other can come out wider than its column. reflow's truncate
// counts ❤️ as one cell and x/ansi's counts 1️⃣ as one; both are two. s
// must be unstyled text: the cut does not skip escape sequences, it
// would count their bytes as cells and could split one.
func Cut(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	if width < 1 {
		return ""
	}
	kept, used := "", 1 // the …
	for g := uniseg.NewGraphemes(s); g.Next(); {
		if used += lipgloss.Width(g.Str()); used > width {
			break
		}
		kept += g.Str()
	}
	return kept + "\u2026"
}
