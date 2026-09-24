package linkpicker

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/ui/styles"
)

// checkbox renders row i's mark column, "" outside multi-select. The
// mark is the x itself; its accent color is decoration only.
func (m *Model) checkbox(i int) string {
	if !m.multiSelect {
		return ""
	}
	muted := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted)
	if !m.marked[i] {
		return muted.Render("[ ] ")
	}
	x := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Accent).Render("x")
	return muted.Render("[") + x + muted.Render("] ")
}

// withMarkedCounter right-aligns an "N marked" counter on the rendered
// title line once anything is marked.
func (m *Model) withMarkedCounter(title string, innerWidth int) string {
	n := len(m.Marked())
	if n == 0 {
		return title
	}
	style := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted)
	counter := style.Render(fmt.Sprintf("%d marked", n))
	gap := innerWidth - lipgloss.Width(title) - lipgloss.Width(counter)
	if gap < 1 {
		gap = 1
	}
	return title + style.Render(strings.Repeat(" ", gap)) + counter
}

// footerText swaps the key hints for the marking ones in multi-select.
// "j/k move" is dropped and the separators are two spaces, not the
// single-choice footer's three, so the line fits an 80-column terminal.
func (m *Model) footerText(single string) string {
	if !m.multiSelect {
		return single
	}
	return "space mark  a all  enter open  esc/q close"
}
