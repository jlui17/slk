package appshortcuts

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
)

type phase int

const (
	phaseClosed phase = iota
	phaseLoading
	phaseMenu
	phaseLoadFailed
	phaseWaiting
	phaseForm
)

// Model is the overlay: the shortcut menu, then the wait for the app,
// then the app's form.
type Model struct {
	phase     phase
	shortcuts []Shortcut
	nameW     int // the widest shortcut name
	selected  int
	top       int // the first menu row in view
	waitFor   string
	form      *Form
}

// OpenLoading shows the menu while the workspace's shortcuts load.
func (m *Model) OpenLoading() {
	*m = Model{phase: phaseLoading}
}

// OpenMenu shows the menu with the workspace's shortcuts.
func (m *Model) OpenMenu(shortcuts []Shortcut) {
	*m = Model{}
	m.SetShortcuts(shortcuts)
}

// SetShortcuts fills a menu that was loading.
func (m *Model) SetShortcuts(shortcuts []Shortcut) {
	m.phase, m.shortcuts, m.selected, m.nameW = phaseMenu, shortcuts, 0, 0
	for _, s := range shortcuts {
		m.nameW = max(m.nameW, lipgloss.Width(s.Name))
	}
}

// SetLoadFailed tells the menu the shortcuts didn't load.
func (m *Model) SetLoadFailed() { m.phase = phaseLoadFailed }

// Wait shows that appName was asked to run its shortcut.
func (m *Model) Wait(appName string) { m.phase, m.waitFor = phaseWaiting, appName }

// OpenForm replaces the menu with the app's form.
func (m *Model) OpenForm(f *Form) { m.phase, m.form = phaseForm, f }

func (m *Model) Close()         { *m = Model{} }
func (m Model) IsVisible() bool { return m.phase != phaseClosed }
func (m Model) Loading() bool   { return m.phase == phaseLoading }

// Listing reports whether the menu waits for its shortcuts: loading, or
// failed while another request may still bring them.
func (m Model) Listing() bool { return m.phase == phaseLoading || m.phase == phaseLoadFailed }
func (m Model) InMenu() bool  { return m.phase == phaseMenu }
func (m Model) Waiting() bool { return m.phase == phaseWaiting }

// Form is the open form, or nil.
func (m Model) Form() *Form {
	if m.phase != phaseForm {
		return nil
	}
	return m.form
}

// MenuKey moves the menu's selection, scrolling a menu taller than
// termHeight just far enough to keep it in view; enter returns the
// shortcut to run.
func (m *Model) MenuKey(k string, termHeight int) (Shortcut, bool) {
	switch k {
	case "up", "k":
		if m.selected > 0 {
			m.selected--
		}
	case "down", "j":
		if m.selected < len(m.shortcuts)-1 {
			m.selected++
		}
	case "enter":
		if len(m.shortcuts) > 0 {
			return m.shortcuts[m.selected], true
		}
	}
	m.top = m.windowTop(termHeight)
	return Shortcut{}, false
}

// maxMenuRows caps the menu's rows on a tall terminal, as the link
// picker's window does.
const maxMenuRows = 20

// menuChromeRows is every line of the menu box but its rows (border,
// title, blank), plus one free terminal line above and below it.
const menuChromeRows = 6

func (m Model) menuRows(termHeight int) int {
	return max(min(len(m.shortcuts), maxMenuRows, termHeight-menuChromeRows), 1)
}

// windowTop is the first row in view: where it was, moved just far
// enough to hold the selection.
func (m Model) windowTop(termHeight int) int {
	rows := m.menuRows(termHeight)
	return max(min(m.top, m.selected, len(m.shortcuts)-rows), m.selected-rows+1, 0)
}

// boxWidth is the overlay's outer width: up to 80 columns, never wider
// than the terminal.
func boxWidth(termWidth int) int {
	return max(min(termWidth-2, 80), 12)
}

// ViewOverlay draws the overlay centered over background.
func (m Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.IsVisible() {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, m.box(termWidth, termHeight), 0.5)
}

func (m Model) box(termWidth, termHeight int) string {
	outer := boxWidth(termWidth)
	inner := outer - 4 // border and one column of padding on each side
	var lines []string
	if f := m.Form(); f != nil {
		lines = m.formLines(f, inner, termHeight-4)
	} else {
		lines = m.menuLines(inner, termHeight)
	}
	content := messages.ReapplyBgAfterResets(strings.Join(lines, "\n"), messages.BgANSI()+messages.FgANSI())
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(styles.Background).
		Background(styles.Background).
		Padding(0, 1).
		Width(outer).
		Render(content)
}

func (m Model) menuLines(w, termHeight int) []string {
	title := "Message actions"
	top, rows := m.windowTop(termHeight), m.menuRows(termHeight)
	position := ""
	if m.phase == phaseMenu && rows < len(m.shortcuts) {
		position = fmt.Sprintf("%d–%d of %d", top+1, top+rows, len(m.shortcuts))
	}
	gap := w - lipgloss.Width(title) - lipgloss.Width(position)
	if gap < 1 {
		position, gap = "", 0
	}
	lines := []string{bold().Foreground(styles.Primary).Render(cellwidth.Cut(title, w)) + plain().Render(strings.Repeat(" ", gap)) + muted().Render(position), ""}
	note := func(s string) []string { return append(lines, muted().Italic(true).Render(cellwidth.Cut(s, w))) }
	switch m.phase {
	case phaseLoading:
		return note("Loading shortcuts…")
	case phaseLoadFailed:
		return note("Could not load shortcuts")
	case phaseWaiting:
		return note("Waiting for " + m.waitFor + "…")
	}
	if len(m.shortcuts) == 0 {
		return note("No app shortcuts in this workspace")
	}
	nameW := min(m.nameW, (w-2)*2/3)
	sep, appW := "   ", w-2-nameW-3
	if appW < 1 {
		sep, appW = "", 0
	}
	for i := top; i < top+rows; i++ {
		s := m.shortcuts[i]
		name := cellwidth.Cut(s.Name, nameW)
		name += strings.Repeat(" ", max(nameW-lipgloss.Width(name), 0))
		app := cellwidth.Cut(s.AppName, appW)
		marker, nameStyle := "  ", plain()
		if i == m.selected {
			marker, nameStyle = "▌ ", bold().Foreground(styles.Primary)
		}
		lines = append(lines, plain().Render(marker)+nameStyle.Render(name)+plain().Render(sep)+muted().Render(app))
	}
	return lines
}

// formLines draws the form in at most maxRows rows: the title and the
// footer stay, and the body scrolls to keep the focused input in view.
func (m Model) formLines(f *Form, w, maxRows int) []string {
	head := styled(bold().Foreground(styles.Primary), wrap(f.view.Title, w))
	head = append(head, "")
	foot := append([]string{""}, f.footer(w)...)
	body, top, end := f.body(w)
	room := max(maxRows-len(head)-len(foot), 1)
	if len(body) > room {
		start := min(max(end-room, 0), top)
		body = body[start:min(start+room, len(body))]
	}
	lines := append(head, body...)
	return append(lines, foot...)
}
