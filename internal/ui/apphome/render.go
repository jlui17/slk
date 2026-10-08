package apphome

import (
	"cmp"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/scrollbar"
	"github.com/gammons/slk/internal/ui/styles"
)

// narrowWidth is blockkit's narrowBreakpoint: below it a section's
// accessory goes under its text instead of beside it.
const narrowWidth = 60

// Pane rows around the body: header, tab row and a blank above it, a
// blank and the key hint below it.
const (
	bodyTop    = 3
	chromeRows = 5
)

// Params is what a render depends on besides the model.
type Params struct {
	// Width and Height are the pane's inner size.
	Width, Height int
	PaneFocused   bool
	InHerdr       bool
	Spinner       string
	// ThemeVersion is styles.Version(): a theme switch redraws.
	ThemeVersion int64
	// Ctx renders the blocks' text, and CtxVersion changes whenever
	// the names or emoji it draws may have: the body is laid out again.
	Ctx        blockkit.Context
	CtxVersion int64
}

type cacheKey struct {
	version, width, height, top int
	paneFocused, inHerdr        bool
	theme, ctx                  int64
	spinner                     string
}

type renderCache struct {
	key  cacheKey
	out  string
	body []bodyLine
}

// bodyLine is one row of the body and the stops it belongs to.
type bodyLine struct {
	text  string
	stops []int
}

func plain() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextPrimary).Background(styles.Background)
}
func muted() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.Background)
}
func control() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMuted).Background(styles.SurfaceDark)
}
func cursor() lipgloss.Style { return plain().Reverse(true).Bold(true) }

// buttonStyle is a button's look by its Block Kit style; danger draws
// in the warning color, as slk never pairs red with green.
func buttonStyle(style string) lipgloss.Style {
	switch style {
	case "primary":
		return control().Foreground(styles.Accent).Bold(true)
	case "danger":
		return control().Foreground(styles.Warning).Bold(true)
	}
	return control()
}

// pad fits s to exactly w cells.
func pad(s string, w int) string {
	n := lipgloss.Width(s)
	if n > w {
		return ansi.Truncate(s, w, "")
	}
	return s + plain().Render(strings.Repeat(" ", w-n))
}

// HeaderLine is the pane's first row, drawn as the messages pane draws
// a channel's: " ▣ Colony".
func HeaderLine(appName string, width int) string {
	return lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.TextPrimary).
		Bold(true).
		Padding(0, 1).
		Render(messages.ChannelGlyph("app") + " " + appName)
}

func activeTab() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Background(styles.Background).Bold(true).Underline(true)
}

// TabRow is the row under the header: "Home   Messages" with the shown
// tab marked, and "m home" at the right on the Messages tab.
func TabRow(width int, home bool) string {
	active := activeTab()
	h, msgs := active.Render("Home"), muted().Render("Messages")
	right := ""
	if !home {
		h, msgs = muted().Render("Home"), active.Render("Messages")
		right = muted().Render("m home")
	}
	left := plain().Render(" ") + h + plain().Render("   ") + msgs
	return pad(pad(left, width-lipgloss.Width(right)-1)+right, width)
}

// BodyHeight is how many rows of the body a pane of inner height h
// shows.
func BodyHeight(h int) int { return max(h, chromeRows+1) - chromeRows }

// Render draws the Home tab's pane, inside its border.
func (m *Model) Render(p Params) string {
	w, h := max(p.Width, 1), max(p.Height, chromeRows+1)
	bodyH := BodyHeight(h)
	iw := max(w-3, 1) // the ▌ column, and a blank column and the scrollbar at the right
	key := cacheKey{m.version, w, h, m.top, p.PaneFocused, p.InHerdr, p.ThemeVersion, p.CtxVersion, p.Spinner}
	if m.cache.out != "" && m.cache.key == key && !m.snap {
		return m.cache.out
	}
	if c := m.cache.key; m.cache.body == nil || c.version != m.version || c.width != w || c.theme != p.ThemeVersion || c.ctx != p.CtxVersion {
		m.cache.body = m.body(p.Ctx, iw)
	}
	body := m.cache.body
	m.scrollInto(body, bodyH)
	key.top = m.top

	tabs := TabRow(w, true)
	if m.homeOnly {
		tabs = pad(plain().Render(" ")+activeTab().Render("Home"), w)
	}
	rows := []string{HeaderLine(m.appName, w), tabs, pad("", w)}
	switch {
	case m.loading:
		rows = append(rows, pad(muted().Italic(true).Render(" "+p.Spinner+" Loading "+m.appName+"'s Home…"), w))
	default:
		var lines []string
		for j := 0; j < bodyH && m.top+j < len(body); j++ {
			l := body[m.top+j]
			bar := plain().Render(" ")
			if m.focus >= 0 && slices.Contains(l.stops, m.focus) {
				c := styles.TextMuted
				if p.PaneFocused {
					c = styles.Accent
				}
				bar = lipgloss.NewStyle().Foreground(c).Background(styles.Background).Render("▌")
			}
			lines = append(lines, bar+pad(l.text, iw)+plain().Render("  "))
		}
		rows = append(rows, scrollbar.Overlay(lines, w, len(body), m.top, bodyH, styles.Background, styles.Border, styles.TextMuted)...)
	}
	for len(rows) < bodyTop+bodyH {
		rows = append(rows, pad("", w))
	}
	rows = append(rows, pad("", w), pad(muted().Render(" "+m.Hint(p.InHerdr)), w))
	out := messages.ReapplyBgAfterResets(strings.Join(rows, "\n"), messages.BgANSI())
	m.cache.key, m.cache.out = key, out
	return out
}

// scrollInto clamps the scroll, and on a focus move scrolls the focused
// stop into view; the first stop shows the blocks above it too.
func (m *Model) scrollInto(body []bodyLine, bodyH int) {
	if m.snap && m.focus >= 0 {
		if m.focus == 0 {
			m.top = 0
		}
		first, last := -1, -1
		for i, l := range body {
			if slices.Contains(l.stops, m.focus) {
				if first < 0 {
					first = i
				}
				last = i
			}
		}
		if first >= 0 {
			if first < m.top {
				m.top = first
			}
			if last >= m.top+bodyH {
				m.top = last - bodyH + 1
			}
		}
	}
	m.snap = false
	m.top = max(0, min(m.top, len(body)-bodyH))
}

// body lays the view's blocks out at width w. A section with an
// accessory gets a blank row above and below it, and a divider one
// above it unless it follows a header, which is close to how Slack
// spaces a Home tab.
func (m *Model) body(ctx blockkit.Context, w int) []bodyLine {
	if m.view == nil {
		return nil
	}
	var out []bodyLine
	blankAfter := false
	blank := func() {
		if len(out) > 0 && out[len(out)-1].text != "" {
			out = append(out, bodyLine{})
		}
	}
	stopIdx := 0
	for i, b := range m.view.blocks {
		var lines []bodyLine
		switch {
		case b.typ == "section" && b.accessory != nil:
			blank()
			lines = m.sectionLines(ctx, b, stopIdx, w)
			stopIdx++
		case b.typ == "actions" && len(b.elements) > 0:
			if blankAfter {
				blank()
			}
			lines = m.actionsLines(b, stopIdx, w)
			stopIdx += len(b.elements)
		default:
			if blankAfter || b.typ == "divider" && i > 0 && m.view.blocks[i-1].typ != "header" {
				blank()
			}
			for _, l := range blockkit.Render([]blockkit.Block{b.parsed}, ctx, w).Lines {
				lines = append(lines, bodyLine{text: l})
			}
		}
		out = append(out, lines...)
		blankAfter = b.typ == "section" && b.accessory != nil
	}
	return out
}

func (m *Model) sectionLines(ctx blockkit.Context, b block, si int, w int) []bodyLine {
	sec, _ := b.parsed.(blockkit.SectionBlock)
	open := m.box == boxSelect && m.focus == si
	label := m.label(si, b.accessory, false)
	accW := lipgloss.Width(label)
	beside := !open && w >= narrowWidth && w-accW-2 >= 10
	textW := w
	if beside {
		textW = w - accW - 2
	}
	var text []string
	if sec.Text != "" {
		text = blockkit.Render([]blockkit.Block{blockkit.SectionBlock{Text: sec.Text}}, ctx, textW).Lines
	}
	var rows []string
	switch {
	case beside && len(text) > 0:
		rows = append(rows, pad(text[0], textW)+plain().Render("  ")+label)
		rows = append(rows, text[1:]...)
	case open:
		rows = append(append(rows, text...), m.optionRows(b.accessory, si, w)...)
	default:
		rows = append(append(rows, text...), label)
	}
	if len(sec.Fields) > 0 {
		rows = append(rows, blockkit.Render([]blockkit.Block{blockkit.SectionBlock{Fields: sec.Fields}}, ctx, w).Lines...)
	}
	return toLines(rows, si)
}

// actionsLines lays an actions block's elements out in rows, two cells
// apart, as blockkit does; the row holding an open select shows its
// options instead.
func (m *Model) actionsLines(b block, first int, w int) []bodyLine {
	var out []bodyLine
	var cur bodyLine
	curW := 0
	flush := func() {
		if len(cur.stops) > 0 {
			out = append(out, cur)
		}
		cur, curW = bodyLine{}, 0
	}
	for j := range b.elements {
		si := first + j
		if m.box == boxSelect && m.focus == si {
			flush()
			out = append(out, toLines(m.optionRows(&b.elements[j], si, w), si)...)
			continue
		}
		label := m.label(si, &b.elements[j], m.focus == si)
		lw := lipgloss.Width(label)
		if curW > 0 && curW+2+lw > w {
			flush()
		}
		if curW > 0 {
			cur.text += plain().Render("  ")
			curW += 2
		}
		cur.text += label
		curW += lw
		cur.stops = append(cur.stops, si)
	}
	flush()
	return out
}

func toLines(rows []string, si int) []bodyLine {
	out := make([]bodyLine, len(rows))
	for i, r := range rows {
		out[i] = bodyLine{text: r, stops: []int{si}}
	}
	return out
}

// label draws an element: a button "[ Start review ]", a select
// "Newest first ▾", or "Waiting for Colony…" while a click on it waits
// for the app's redraw. withCursor draws it in reverse video.
func (m *Model) label(si int, e *element, withCursor bool) string {
	if m.wait != nil && m.stopOf(m.wait.key) == si {
		return muted().Italic(true).Render("Waiting for " + m.appName + "…")
	}
	text := cmp.Or(e.Text.String(), e.Placeholder.String())
	if e.Type == "static_select" {
		if i := m.chosenIndex(m.stops[si], e); i >= 0 {
			text = e.Options[i].Text.Text
		}
	}
	t := blockkit.ControlText(e.Type, text)
	if withCursor {
		return cursor().Render(t)
	}
	return buttonStyle(e.Style).Render(t)
}

// optionRows are an open select's options, "(•) Newest first" for the
// chosen one, the one under the cursor in reverse video.
func (m *Model) optionRows(e *element, si int, w int) []string {
	chosen := m.chosenIndex(m.stops[si], e)
	rows := make([]string, len(e.Options))
	for j, o := range e.Options {
		mark := "( ) "
		if j == chosen {
			mark = "(•) "
		}
		st := plain()
		if j == m.selectCursor {
			st = cursor()
		}
		rows[j] = st.Render(cellwidth.Cut(mark+o.Text.Text, w))
	}
	return rows
}

// ConfirmBox is the open confirm dialog, "" when none is open.
func (m *Model) ConfirmBox(termWidth int) string {
	if m.box != boxConfirm {
		return ""
	}
	w := max(min(66, termWidth-8), 20)
	inner := w - 4
	c := m.confirm
	title := lipgloss.NewStyle().Foreground(styles.Primary).Background(styles.Background).Bold(true).Render(c.TitleText())
	content := title + "\n\n"
	if c.BodyText() != "" {
		content += plain().Render(messages.WordWrap(c.BodyText(), inner)) + "\n\n"
	}
	content += muted().Render("enter " + c.ConfirmText() + "  esc " + c.DenyText())
	content = messages.ReapplyBgAfterResets(content, messages.BgANSI()+messages.FgANSI())
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(styles.Background).
		Background(styles.Background).
		Padding(1, 1).
		Width(w).
		Render(content)
}

// ClickAt handles a click at row y and column x of the pane's inner
// area: the link drawn there, if any, is returned to open; else the
// focus moves to the stop on that row.
func (m *Model) ClickAt(x, y int) string {
	i := m.top + y - bodyTop
	bodyH := BodyHeight(m.cache.key.height)
	if m.box != boxNone || y < bodyTop || y >= bodyTop+bodyH || i >= len(m.cache.body) {
		return ""
	}
	l := m.cache.body[i]
	if url := messages.LinkAt(l.text, x-1); url != "" {
		return url
	}
	if len(l.stops) > 0 {
		m.focus = l.stops[0]
		m.dirty()
	}
	return ""
}
