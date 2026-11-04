package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// threadReplyBox is the thread reply box's width and how far into the
// pane it starts: as wide as the replies' text (messages.TextWidth)
// plus the box's border and padding, with its text in the replies'
// text column. A pane too narrow for that gets the full width.
func threadReplyBox(paneWidth int) (width, indent int) {
	// The box draws its border and one padding cell left of its text,
	// and one padding cell right of it (styles.ComposeBox).
	indent = messages.TextCol - 2
	width = messages.TextWidth(paneWidth) + 3
	if indent+width > paneWidth {
		return paneWidth, 0
	}
	return width, indent
}

// indentRows puts every row of view indent cells in and pads it to
// width, in the pane background.
func indentRows(view string, indent, width int) string {
	if indent == 0 {
		return view
	}
	bg := lipgloss.NewStyle().Background(styles.Background)
	lead := bg.Render(strings.Repeat(" ", indent))
	rows := strings.Split(view, "\n")
	for i, r := range rows {
		r = lead + r
		if pad := width - ansi.StringWidth(r); pad > 0 {
			r += bg.Render(strings.Repeat(" ", pad))
		}
		rows[i] = r
	}
	return strings.Join(rows, "\n")
}
