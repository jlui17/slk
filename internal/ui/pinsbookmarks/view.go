package pinsbookmarks

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

// maxRows is the most rows a section shows; a longer one scrolls.
const maxRows = 10

// boxWidth is the dialog's outer width on a terminal termWidth wide.
func boxWidth(termWidth int) int {
	return max(min(max(termWidth*6/10, 60), 92, termWidth-2), 1)
}

// pick is the item a line of the dialog's body draws.
type pick struct {
	section Section
	index   int
}

// layout is the dialog laid out for one terminal size: its lines inside
// the box, and the item each row line of the body draws, by body line.
type layout struct {
	termWidth, termHeight int
	lines                 []string
	picks                 map[int]pick
	hasInput              bool
}

// line is one line of a section's list: a row, or a folder's heading
// (index -1).
type line struct {
	index  int
	folder string
}

// lay lays the dialog out, or returns the layout already made for this
// size since the last change.
func (m *Model) lay(st chrome.Styles, termWidth, termHeight int) *layout {
	if l := m.laid; l != nil && l.termWidth == termWidth && l.termHeight == termHeight {
		return l
	}
	inner := boxWidth(termWidth) - st.Box.GetHorizontalFrameSize()
	lines := [2][]line{m.sectionLines(Pins), m.sectionLines(Bookmarks)}
	// The lines that are not section rows (the head, the body, the blank
	// lines around it and the footer), the box's frame, and one free
	// terminal line above and below the box fix how many rows the
	// sections share.
	head, body := m.build(st, inner, lines, [2]int{})
	fixed := len(head) + 1 + len(body) + 2 + st.Box.GetVerticalFrameSize() + 2
	pins, bookmarks := windowSizes(len(lines[Pins]), len(lines[Bookmarks]), termHeight-fixed)
	head, body = m.build(st, inner, lines, [2]int{pins, bookmarks})
	l := &layout{termWidth: termWidth, termHeight: termHeight, picks: map[int]pick{}, hasInput: len(head) > 1}
	for i, b := range body {
		l.picks[i] = b.pick
	}
	l.lines = head
	l.lines = append(l.lines, "")
	for _, b := range body {
		l.lines = append(l.lines, b.text)
	}
	l.lines = append(l.lines, "", m.footer(st, inner))
	m.laid = l
	return l
}

// bodyLine is one line of the body, and the item it draws, if any.
type bodyLine struct {
	text string
	pick
}

func noPick(text string) bodyLine { return bodyLine{text, pick{index: -1}} }

// build draws the title and filter (head) and the body, each section
// showing rows[s] of its lines.
func (m *Model) build(st chrome.Styles, inner int, lines [2][]line, rows [2]int) (head []string, body []bodyLine) {
	title := "Pins and bookmarks · " + m.title
	if m.filter != "" {
		n := len(m.lists[Pins].shown) + len(m.lists[Bookmarks].shown)
		matches := strconv.Itoa(n) + " matches"
		if n == 1 {
			matches = "1 match"
		}
		title = cellwidth.Cut(title, inner-lipgloss.Width(matches)-1)
		head = append(head, rightAlign(st.Title.Render(title), st.Footer.Render(matches), inner))
	} else {
		head = append(head, st.Title.Render(cellwidth.Cut(title, inner)))
	}
	if m.filter != "" || m.filtering {
		head = append(head, chrome.FilterInput(st, m.filter, "filter pins and bookmarks"))
	}

	pins, bookmarks := m.lists[Pins], m.lists[Bookmarks]
	switch {
	case pins.loading && bookmarks.loading:
		return head, []bodyLine{noPick(st.Empty.Render("Loading pins and bookmarks…"))}
	case pins.err != "" && pins.err == bookmarks.err:
		return head, m.errorLines(st, inner, "pins and bookmarks", pins.err)
	}
	for _, s := range []Section{Pins, Bookmarks} {
		body = append(body, m.drawSection(st, s, lines[s], rows[s], inner)...)
		body = append(body, noPick(""))
	}
	if len(m.pins) == 0 && len(m.bookmarks) == 0 && !pins.loading && !bookmarks.loading && pins.err == "" && bookmarks.err == "" {
		body = append(body, noPick(st.Footer.Render("Pins and bookmarks added in the Slack app show up here.")))
	} else {
		body = body[:len(body)-1]
	}
	return head, body
}

func (m *Model) errorLines(st chrome.Styles, inner int, what, err string) []bodyLine {
	warning := lipgloss.NewStyle().Background(styles.Background).Foreground(styles.Warning).Bold(true)
	out := []bodyLine{noPick(warning.Render(cellwidth.Cut("! Couldn't load "+what+" for "+m.title, inner)))}
	for _, l := range strings.Split(messages.WordWrap(err, inner), "\n") {
		out = append(out, noPick(st.Footer.Render(l)))
	}
	return out
}

// sectionLines is the lines of section s's rows the filter shows, a
// folder's heading before its first link.
func (m *Model) sectionLines(s Section) []line {
	var out []line
	folder := ""
	for _, i := range m.lists[s].shown {
		if s == Bookmarks && m.bookmarks[i].Folder != folder {
			folder = m.bookmarks[i].Folder
			if folder != "" {
				out = append(out, line{index: -1, folder: folder})
			}
		}
		out = append(out, line{index: i})
	}
	return out
}

// windowSizes is how many lines each section's window shows in room
// rows: up to maxRows, both shrunk, the longer first, to fit.
func windowSizes(pins, bookmarks, room int) (int, int) {
	p, b := min(pins, maxRows), min(bookmarks, maxRows)
	room = max(room, 2)
	for p+b > room && (p > 1 || b > 1) {
		if p > b {
			p--
		} else {
			b--
		}
	}
	return p, b
}

// drawSection draws section s: its heading, then the window of its lines
// that holds the cursor, or why it has none.
func (m *Model) drawSection(st chrome.Styles, s Section, lines []line, rows, inner int) []bodyLine {
	l := m.lists[s]
	name, what, empty := "Pins", "pins", "Nothing is pinned in "+m.title+"."
	if s == Bookmarks {
		name, what, empty = "Bookmarks", "bookmarks", "No bookmarks in "+m.title+"."
	}
	nameStyle := st.Footer.Bold(true)
	if s == m.section {
		nameStyle = st.Row.Bold(true)
	}
	heading := nameStyle.Render(name)
	if l.loading || l.err != "" {
		out := []bodyLine{noPick(heading)}
		if l.loading {
			return append(out, noPick(st.Empty.Render(" Loading "+what+"…")))
		}
		return append(out, m.errorLines(st, inner, what, l.err)...)
	}
	count := strconv.Itoa(len(l.folded))
	if m.filter != "" {
		count = fmt.Sprintf("%d of %d", len(l.shown), len(l.folded))
	}
	heading += st.Footer.Render("  " + count)

	selectedLine := -1
	if s == m.section {
		selectedLine = slices.IndexFunc(lines, func(ln line) bool { return ln.index == m.selected[s] })
	}
	start, end := chrome.Window(max(selectedLine, 0), len(lines), rows)
	list := chrome.List{Start: start, End: end, Total: len(lines)}
	if list.Scrolls() {
		before, in := 0, 0
		for k, ln := range lines[:end] {
			if ln.index >= 0 && k < start {
				before++
			} else if ln.index >= 0 {
				in++
			}
		}
		heading = rightAlign(heading, st.Footer.Render(fmt.Sprintf("%d–%d of %d", before+1, before+in, len(l.shown))), inner)
	}
	out := []bodyLine{noPick(heading)}
	switch {
	case len(l.folded) == 0:
		out = append(out, noPick(st.Empty.Render(" "+empty)))
	case len(lines) == 0:
		out = append(out, noPick(st.Empty.Render(" No matches")))
	}
	width := list.ContentWidth(inner)
	for k := start; k < end; k++ {
		ln := lines[k]
		var content string
		switch {
		case ln.index < 0:
			content = st.Footer.Bold(true).Render(cellwidth.Cut(ln.folder, width))
		case s == Pins:
			content = m.pinContent(st, m.pins[ln.index], k == selectedLine, width)
		default:
			content = m.bookmarkContent(st, m.bookmarks[ln.index], k == selectedLine, width)
		}
		row := chrome.Marker(st, k == selectedLine) + pad(content, width) + list.Gutter(st, k)
		if ln.index < 0 {
			out = append(out, noPick(row))
		} else {
			out = append(out, bodyLine{row, pick{s, ln.index}})
		}
	}
	return out
}

const inThread = "  in thread"

func (m *Model) pinContent(st chrome.Styles, p PinRow, selected bool, width int) string {
	c := m.pinCols
	text, textStyle := st.Row, st.Row
	if p.File {
		textStyle = st.Row.Foreground(styles.Primary)
	}
	if selected {
		text, textStyle = st.RowSelected, st.RowSelected
	}
	right := ""
	if c.thread {
		right = strings.Repeat(" ", lipgloss.Width(inThread))
		if p.InThread {
			right = inThread
		}
	}
	body := p.Text
	if p.File {
		body = "[File] " + body
	}
	room := max(width-c.author-c.day-lipgloss.Width(right), 1)
	return text.Render(pad(cellwidth.Cut(p.Author, c.author-2), c.author)) +
		st.Footer.Render(pad(p.Day, c.day)) +
		textStyle.Render(pad(cellwidth.Cut(body, room), room)) +
		st.Footer.Render(right)
}

const badge = " [slk]"

func (m *Model) bookmarkContent(st chrome.Styles, b BookmarkRow, selected bool, width int) string {
	title := st.Row
	if selected {
		title = st.RowSelected
	}
	indent := ""
	if b.Folder != "" {
		indent = "  "
	}
	mark := ""
	if b.InApp {
		mark = badge
	}
	room := max(width-lipgloss.Width(mark), 1)
	head := cellwidth.Cut(indent+b.Title, room)
	site := cellwidth.Cut("  "+b.Site, max(room-lipgloss.Width(head), 0))
	return title.Render(head) + st.Footer.Render(pad(site, room-lipgloss.Width(head))) + styles.InAppBadge().Render(mark)
}

// footer is the key hints for the dialog's state, with the [slk]
// legend when a bookmark shows the badge.
func (m *Model) footer(st chrome.Styles, inner int) string {
	pins, bookmarks := m.lists[Pins], m.lists[Bookmarks]
	text := "tab pins/bookmarks  j/k move  enter open  / filter  esc close"
	if pins.err != "" || bookmarks.err != "" {
		text = "tab pins/bookmarks  j/k move  enter open  / filter  r retry  esc close"
	}
	switch {
	case pins.loading && bookmarks.loading:
		text = "esc close"
	case pins.err != "" && pins.err == bookmarks.err:
		text = "r retry  esc close"
	case m.filtering:
		text = "↑/↓ move  enter open  esc clear"
	case len(pins.folded) == 0 && len(bookmarks.folded) == 0 && pins.err == "" && bookmarks.err == "" && !pins.loading && !bookmarks.loading:
		text = "esc close"
	case slices.ContainsFunc(bookmarks.shown, func(i int) bool { return m.bookmarks[i].InApp }):
		legend := styles.InAppBadge().Render("[slk]") + st.Footer.Render(" opens in slk")
		if lipgloss.Width(text)+2+lipgloss.Width(legend) <= inner {
			return st.Footer.Render(text+"  ") + legend
		}
	}
	return st.Footer.Render(cellwidth.Cut(text, inner))
}

// pad fills s with spaces to width cells.
func pad(s string, width int) string {
	return s + strings.Repeat(" ", max(width-lipgloss.Width(s), 0))
}

// rightAlign puts right at the end of a line inner cells wide that
// starts with left.
func rightAlign(left, right string, inner int) string {
	gap := max(inner-lipgloss.Width(left)-lipgloss.Width(right), 1)
	return left + strings.Repeat(" ", gap) + right
}

// BoxSize returns the outer size of the dialog's box.
func (m *Model) BoxSize(termWidth, termHeight int) (int, int) {
	st := messages.ChromeStyles()
	return boxWidth(termWidth), len(m.lay(st, termWidth, termHeight).lines) + st.Box.GetVerticalFrameSize()
}

// ClickRow puts the cursor on the row at box-local localY (0 is the top
// border) and reports whether a row was there.
func (m *Model) ClickRow(termWidth, termHeight, localY int) bool {
	st := messages.ChromeStyles()
	l := m.lay(st, termWidth, termHeight)
	p, ok := l.picks[localY-chrome.BodyTop(st, l.hasInput)]
	if !ok || p.index < 0 {
		return false
	}
	m.laid = nil
	m.section, m.selected[p.section] = p.section, p.index
	return true
}

// View renders the dialog's box, without the dimmed screen under it.
func (m *Model) View(termWidth, termHeight int) string {
	st := messages.ChromeStyles()
	return chrome.Box(st, strings.Join(m.lay(st, termWidth, termHeight).lines, "\n"), boxWidth(termWidth))
}

// ViewOverlay renders the dialog centered over a dimmed background.
func (m *Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, m.View(termWidth, termHeight), 0.5)
}
