package linkpicker

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/rivo/uniseg"
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

// maxWindowRows caps the scroll window on a tall terminal. The app
// fetches a message preview per row in view, so it also bounds how many
// Slack calls opening the picker makes at once.
const maxWindowRows = 20

// boxChromeRows is every line of the box but the item rows (border,
// padding, title, filter line, blank, footer), plus one free terminal
// line above and below it.
const boxChromeRows = 10

// maxBoxWidth is how far widenForRows grows the box on a wide terminal.
const maxBoxWidth = 160

// widenForRows grows the box until the widest row fits, as far as
// maxBoxWidth and the terminal allow. It goes by the rows as they were
// at Open, so a preview landing never resizes the box.
func (m *Model) widenForRows(overlayWidth, termWidth int) int {
	need := 1 + lipgloss.Width(m.checkbox(0)) + m.rowWidth + lipgloss.Width(" [slk]") + 4
	return max(overlayWidth, min(need, maxBoxWidth, termWidth-2))
}

// The Side column takes a third of what a row has for its text and its
// Side, between sideMinWidth and sideMaxWidth cells. Under sideMinRoom
// cells for the two it is not drawn: the text matters more.
const (
	sideMinWidth = 16
	sideMaxWidth = 40
	sideMinRoom  = 40
	sideGap      = 2
)

// sideWidth depends only on what is fixed when the picker opens (the
// box, the labels, the checkbox), so the column is where it was when a
// preview lands.
func (m *Model) sideWidth(innerWidth int) int {
	label := 0
	for _, it := range m.items {
		label = max(label, lipgloss.Width(it.Label)+2)
	}
	room := innerWidth - 1 - lipgloss.Width(m.checkbox(0)) - label - lipgloss.Width(" [slk]")
	if room < sideMinRoom {
		return 0
	}
	return min(max(room/3, sideMinWidth), sideMaxWidth)
}

// drawnText is rowText, but for a row whose Side column is not drawn
// for want of room and whose preview has not landed: its fallback text
// says when, and without its Side nothing says where.
func (m *Model) drawnText(it Item, innerWidth int) string {
	if it.Side != "" && !it.previewed && m.sideWidth(innerWidth) == 0 {
		it.Display = it.Side + " \u00b7 " + it.Display
	}
	return rowText(it)
}

// sideColumn is the Side column of row it as drawn, gap included: ""
// for a row without a Side, whose text may run on to the badge. A row
// without the [slk] badge keeps the badge's cells empty, so the column
// lines up with the rows that have it.
func (m *Model) sideColumn(it Item, innerWidth int) string {
	width := m.sideWidth(innerWidth)
	if it.Side == "" || width == 0 {
		return ""
	}
	side := Cut(it.Side, width)
	if !it.InApp {
		width += lipgloss.Width(" [slk]")
	}
	side = strings.Repeat(" ", sideGap) + side + strings.Repeat(" ", max(width-lipgloss.Width(side), 0))
	return lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted).Render(side)
}

// withTitleStatus right-aligns on the rendered title line where the
// scroll window is ("7–18 of 46") when it does not hold every row, and
// an "N marked" counter once anything is marked. Under a filter the
// rows counted are its matches and the line says so ("4 matches",
// "1–14 of 15 matches"), also when they all fit; none is filterLine's
// "no matching rows".
func (m *Model) withTitleStatus(title string, innerWidth int) string {
	var parts []string
	shown, window := m.shown(), m.window()
	position := ""
	if len(window) < len(shown) {
		position = fmt.Sprintf("%d–%d of ", m.top+1, m.top+len(window))
	}
	switch {
	case m.filter == "":
		if position != "" {
			parts = append(parts, position+strconv.Itoa(len(shown)))
		}
	case len(shown) == 1:
		parts = append(parts, "1 match")
	case len(shown) > 1:
		parts = append(parts, fmt.Sprintf("%s%d matches", position, len(shown)))
	}
	if n := len(m.Marked()); n > 0 {
		parts = append(parts, fmt.Sprintf("%d marked", n))
	}
	if len(parts) == 0 {
		return title
	}
	style := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted)
	status := style.Render(strings.Join(parts, " · "))
	gap := innerWidth - lipgloss.Width(title) - lipgloss.Width(status)
	if gap < 1 {
		gap = 1
	}
	return title + style.Render(strings.Repeat(" ", gap)) + status
}

// filterLine is the line under the title: empty until / starts a
// filter, then "/text", with a block cursor while keys go to it.
func (m *Model) filterLine() string {
	if m.filter == "" && !m.filtering {
		return ""
	}
	line := "/" + m.filter
	if m.filtering {
		line += "\u2588"
	}
	line = lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextPrimary).Render(line)
	if len(m.shown()) == 0 {
		line += lipgloss.NewStyle().Background(styles.Background).Foreground(styles.TextMuted).Render("  no matching rows")
	}
	return line
}

// footerText is the key hints for the picker's state. The separators
// are two spaces, not upstream's three, and "j/k move" is dropped in
// multi-select, so the line fits the box an 80-column terminal gets; a
// narrower box cuts it rather than wrap it into a taller box.
func (m *Model) footerText(innerWidth int) string {
	text := "/ filter  j/k move  enter select  esc close"
	if m.multiSelect {
		text = "/ filter  space mark  a all  enter open  esc close"
	}
	if m.filtering {
		text = "\u2191/\u2193 move  tab to list  enter select  esc clear"
		if m.multiSelect {
			text = "\u2191/\u2193 move  tab mark rows  enter open  esc clear"
		}
	} else if m.filter != "" {
		text = strings.Replace(text, "esc close", "esc clear", 1)
	}
	return Cut(text, innerWidth)
}
