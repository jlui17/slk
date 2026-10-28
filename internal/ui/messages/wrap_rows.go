package messages

import (
	"strings"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/styles"
)

// WrapStyles are the lipgloss styles that turn a rendered message into
// list rows: the fill for the unselected row, the invisible and the
// tinted left border. Build them once per cache build (NewWrapStyles)
// and reuse them for every message, rather than allocating per message.
type WrapStyles struct {
	fill     lipgloss.Style
	tint     lipgloss.Style
	invis    lipgloss.Style
	selected lipgloss.Style
	focused  bool

	// What each style draws around a one-row line (rowAffixOf), so
	// rows are assembled without a lipgloss pass over each message.
	fillRow, tintRow, invisRow, selectedRow rowAffix

	// rowBreaks memoises wrapRowBreak by the escapes in effect.
	rowBreaks map[string]rowAffix

	// stats, when non-nil, collects per-sub-step durations (channel
	// pane perf instrumentation).
	stats *entryPerfStats
}

// NewWrapStyles returns the styles for the current theme; focused picks
// the selection colours.
func NewWrapStyles(focused bool) WrapStyles {
	s := WrapStyles{
		fill: lipgloss.NewStyle().Background(styles.Background),
		tint: lipgloss.NewStyle().Background(styles.SelectionTintColor(focused)),
		invis: lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background),
		selected: lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.SelectionBorderColor(focused)).
			BorderBackground(styles.SelectionTintColor(focused)).
			Background(styles.SelectionTintColor(focused)),
		focused:   focused,
		rowBreaks: map[string]rowAffix{},
	}
	s.fillRow = rowAffixOf(s.fill)
	s.tintRow = rowAffixOf(s.tint)
	s.invisRow = rowAffixOf(s.invis)
	s.selectedRow = rowAffixOf(s.selected)
	return s
}

// SelectedVariant renders content as the selected row: the selection
// tint repainted over inner spans, filled to the row width, then the
// selection border. Split on "\n".
func (s WrapStyles) SelectedVariant(content string, width int) []string {
	tinted := ReapplyBgAfterResets(RepaintBgToSelectionTint(content, s.focused), SelectionTintBgANSI(s.focused)) // fork
	rows := s.fillRows(tinted, width-1, s.tint, s.tintRow)
	return s.selectedRow.around(rows)
}

// WrappedRows are the rows of one message, bordered and filled, ready
// for an entrylist.Entry. A pane adds what is its own: ID, Ref, hits, and
// the channel pane's trailing spacer rows.
type WrappedRows struct {
	// Lines is the unselected row, with the invisible left border.
	Lines []string
	// Selected is the selected row, with the tinted border and a
	// date-qualified header timestamp.
	Selected []string
	// Plain mirrors the unbordered, filled content, so the border column
	// never reaches clipboard text. Pair it with Entry.ColOffset = 1.
	Plain []entrylist.PlainLine
	// ShortSrc is the content the short-timestamp selected variant is
	// built from (EntryPayload.SelectedLines); empty when Selected
	// already carries the short form.
	ShortSrc string
}

// WrapRows fills and borders a rendered message for a content pane's
// list. Only the selected variant carries the date-qualified timestamp:
// Lines and Plain keep the short form, so clipboard text and the
// mouse-to-column mapping are unchanged. The short-timestamp selected
// variant is not built here (it would double the fill and border cost of
// every cache build); ShortSrc lets EntryPayload.SelectedLines build it on
// demand.
func WrapRows(msg MessageItem, r RenderedMessage, width int, s WrapStyles) WrappedRows {
	rendered := r.Content
	renderedSel := SelectedHeader(rendered, r.Header, msg.TS, msg.Timestamp, r.HeaderBudget)

	// Two filled variants: the theme background for the unselected row
	// and the selection tint for the selected one. Without per-variant
	// fills, the trailing whitespace of every wrapped line shows the
	// wrong background and the tint stops at the last character of
	// content. For the selected variant every theme-background escape
	// inside the content is also swapped for the tint (SelectedVariant),
	// or inner spans would show through as dark patches.
	var bwT0 time.Time
	if s.stats != nil {
		bwT0 = time.Now()
	}
	filledNormal := s.fillRows(rendered, width-1, s.fill, s.fillRow)
	normal := s.invisRow.around(filledNormal)
	selected := s.SelectedVariant(renderedSel, width)
	var shortSrc string
	if renderedSel != rendered {
		shortSrc = rendered
	}
	if s.stats != nil {
		s.stats.borderWrapTotal += time.Since(bwT0)
	}

	var plT0 time.Time
	if s.stats != nil {
		plT0 = time.Now()
	}
	plain := entrylist.PlainLines(strings.Join(filledNormal, "\n"))
	if s.stats != nil {
		s.stats.plainLinesTotal += time.Since(plT0)
	}

	return WrappedRows{
		Lines:    normal,
		Selected: selected,
		Plain:    plain,
		ShortSrc: shortSrc,
	}
}

// rowAffix is what a style draws before and after a one-row line.
type rowAffix struct{ pre, suf string }

// rowAffixOf splits st.Render(" ") around its space. For a style that
// sets colours and a left border but no width, padding or margins,
// rendering rows of equal width is each row between pre and suf.
func rowAffixOf(st lipgloss.Style) rowAffix {
	r := st.Render(" ")
	i := strings.Index(r, " ")
	return rowAffix{pre: r[:i], suf: r[i+1:]}
}

// around returns rows with a in front of and behind each.
func (a rowAffix) around(rows []string) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = a.pre + r + a.suf
	}
	return out
}

// fillRows is st.Width(width).Render(content) split on "\n", for a st
// that sets only a background, whose one-row affix is a. Lipgloss first
// wraps content: ansi.Wrap, which changes nothing when every row fits
// and content has no control byte but newline and ESC, then a pass that
// closes the style and link in effect at each row's end and reopens them
// on the next row (wrapRowBreak). It then styles every row and pads it
// with styled spaces. In that case the rows are assembled here, which
// skips lipgloss's byte-by-byte passes and its measuring of every row
// once per pass; otherwise lipgloss renders them.
func (s WrapStyles) fillRows(content string, width int, st lipgloss.Style, a rowAffix) []string {
	rows := strings.Split(content, "\n")
	inEffect, ok := escapesAtRowEnds(content, len(rows))
	widths := make([]int, len(rows))
	for i, r := range rows {
		if !ok {
			break
		}
		widths[i] = ansi.StringWidth(r)
		ok = widths[i] <= width
	}
	if !ok {
		return strings.Split(st.Width(width).Render(content), "\n")
	}
	for i := range rows {
		if i > 0 && inEffect[i-1] != "" {
			rows[i] = s.wrapRowBreak(inEffect[i-1]).suf + rows[i]
		}
		if i < len(rows)-1 && inEffect[i] != "" {
			rows[i] += s.wrapRowBreak(inEffect[i]).pre
		}
	}
	for i, r := range rows {
		row := a.pre + r + a.suf
		if pad := width - widths[i]; pad > 0 {
			row += a.pre + strings.Repeat(" ", pad) + a.suf
		}
		rows[i] = row
	}
	return rows
}

// wrapRowBreak returns what lipgloss's wrap writes before (pre) and
// after (suf) a newline reached with the escapes inEffect in effect:
// the resets of the style and link they leave open, and the sequences
// that reopen them. It asks lipgloss.Wrap, which over escapes alone and
// a newline writes nothing else.
func (s WrapStyles) wrapRowBreak(inEffect string) rowAffix {
	if b, ok := s.rowBreaks[inEffect]; ok {
		return b
	}
	out := lipgloss.Wrap(inEffect+"\n", 1, "")
	nl := strings.LastIndexByte(out, '\n')
	b := rowAffix{pre: out[len(inEffect):nl], suf: out[nl+1:]}
	s.rowBreaks[inEffect] = b
	return b
}

// escapesAtRowEnds returns, for each of the rows of s, the escapes in
// effect at its end: every SGR since the last "\x1b[m" or "\x1b[0m"
// and every OSC since the last "\x1b]8;;" link close, which replayed
// leave lipgloss's wrap in the same state. "" means none. ok is false
// when s is not valid UTF-8, has a control byte but newline and ESC
// (the wrap counts a tab or a carriage return as a cell, where the
// width of a row does not), or an escape other than a CSI or an OSC.
func escapesAtRowEnds(s string, rows int) (inEffect []string, ok bool) {
	if !utf8.ValidString(s) {
		return nil, false
	}
	inEffect = make([]string, 0, rows)
	var sgr, osc strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '\n':
			inEffect = append(inEffect, sgr.String()+osc.String())
		case c == '\x1b':
			end, ok := escapeEnd(s, i)
			if !ok {
				return nil, false
			}
			switch seq := s[i:end]; {
			case seq == "\x1b[m" || seq == "\x1b[0m":
				sgr.Reset()
			case seq == "\x1b]8;;\a" || seq == "\x1b]8;;\x1b\\":
				osc.Reset()
			case seq[1] == ']':
				osc.WriteString(seq)
			case seq[end-i-1] == 'm':
				sgr.WriteString(seq)
			}
			i = end - 1
		case c < 0x20 || c == 0x7f:
			return nil, false
		}
	}
	return append(inEffect, sgr.String()+osc.String()), true
}

// escapeEnd returns the end of the CSI or OSC escape at s[i]. ok is
// false for any other escape, or one cut short.
func escapeEnd(s string, i int) (end int, ok bool) {
	if i+1 == len(s) {
		return 0, false
	}
	j := i + 2
	switch s[i+1] {
	case '[':
		for ; j < len(s); j++ {
			if c := s[j]; c >= 0x40 && c <= 0x7e {
				return j + 1, true
			} else if c < 0x20 {
				return 0, false
			}
		}
	case ']':
		for ; j < len(s); j++ {
			switch c := s[j]; {
			case c == '\a':
				return j + 1, true
			case c == '\x1b':
				return j + 2, j+1 < len(s) && s[j+1] == '\\'
			case c < 0x20:
				return 0, false
			}
		}
	}
	return 0, false
}
