package linkpicker

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
)

// SetDisplay replaces row index's Display text in place (async
// preview fill). Out-of-range indexes are ignored.
func (m *Model) SetDisplay(index int, display string) {
	if index >= 0 && index < len(m.items) {
		m.items[index].Display = display
		m.items[index].previewed = true
	}
}

// SetSide replaces row index's Side in place (async preview fill).
// Out-of-range indexes are ignored.
func (m *Model) SetSide(index int, side string) {
	if index >= 0 && index < len(m.items) {
		m.items[index].Side = side
	}
}

// SetMultiSelect turns row marking on for the picker Open just showed.
// Open and Close turn it back off.
func (m *Model) SetMultiSelect(on bool) { m.multiSelect = on }

// MultiSelect reports whether rows can be marked.
func (m *Model) MultiSelect() bool { return m.multiSelect }

// ToggleMark flips the mark on the highlighted row.
func (m *Model) ToggleMark() {
	if len(m.shown()) == 0 {
		return
	}
	if m.marked == nil {
		m.marked = make(map[int]bool)
	}
	m.marked[m.selected] = !m.marked[m.selected]
}

// ToggleMarkAll clears the mark of every row the filter shows when all
// of them are marked, and marks them all otherwise. Marks on rows the
// filter hides stay.
func (m *Model) ToggleMarkAll() {
	shown := m.shown()
	mark := false
	for _, i := range shown {
		mark = mark || !m.marked[i]
	}
	if m.marked == nil {
		m.marked = make(map[int]bool, len(shown))
	}
	for _, i := range shown {
		m.marked[i] = mark
	}
}

// Marked returns the marked rows in list order, hidden ones included.
func (m *Model) Marked() []Item {
	var out []Item
	for i, it := range m.items {
		if m.marked[i] {
			out = append(out, it)
		}
	}
	return out
}

// resetFork is the fork's half of Open and Close.
func (m *Model) resetFork() {
	m.multiSelect = false
	m.marked = nil
	m.filter = ""
	m.filtering = false
	m.top = 0
	m.rowWidth = 0
	for i, it := range m.items {
		m.rowWidth = max(m.rowWidth, lipgloss.Width(rowText(it))+2+lipgloss.Width(it.Detail))
		if it.FilterText == "" {
			m.items[i].FilterText = rowText(it)
		}
		if it.Side != "" {
			m.rowWidth = maxBoxWidth // a preview of unknown length will land here
		}
	}
}

// SetTermHeight tells the picker how tall the terminal is before its
// first draw, so ItemsInView is right as soon as Open returns. Every
// draw sets it again.
func (m *Model) SetTermHeight(h int) { m.termHeight = h }

// Filtering reports whether typed keys go to the filter line.
func (m *Model) Filtering() bool { return m.filtering }

// ItemsInView returns the rows the scroll window shows.
func (m *Model) ItemsInView() []Item {
	var out []Item
	for _, i := range m.window() {
		out = append(out, m.items[i])
	}
	return out
}

// rowText is the row's main text.
func rowText(it Item) string {
	var parts []string
	if it.Label != "" {
		parts = append(parts, it.Label)
	}
	switch {
	case it.Display != "":
		parts = append(parts, it.Display)
	case it.URL != "" && it.URL != it.Label:
		parts = append(parts, it.URL)
	}
	return strings.Join(parts, "  ")
}

// shown returns the indexes of the rows the filter keeps: those whose
// FilterText contains it, whatever the case, and every row when there
// is no filter. FilterText is fixed at Open, so a preview that lands
// (SetDisplay) changes neither the rows shown nor the highlight. A
// permalink row's opener leaves its date, its preview and its URL out
// of it: the first two change under the filter or are not fetched for
// rows out of view, and the URL is IDs and digits the row never shows.
func (m *Model) shown() []int {
	want := strings.ToLower(m.filter)
	var out []int
	for i, it := range m.items {
		if strings.Contains(strings.ToLower(it.FilterText), want) {
			out = append(out, i)
		}
	}
	return out
}

// window returns the indexes of the rows to draw: as many of shown as
// the terminal has room for, scrolled just far enough to hold the
// highlighted row.
func (m *Model) window() []int {
	shown := m.shown()
	rows := min(len(shown), maxWindowRows)
	if m.termHeight > 0 {
		rows = min(rows, max(m.termHeight-boxChromeRows, 1))
	}
	at := slices.Index(shown, m.selected)
	m.top = max(min(m.top, at, len(shown)-rows), at-rows+1, 0)
	return shown[m.top : m.top+rows]
}

func (m *Model) setFilter(filter string) {
	m.filter = filter
	if shown := m.shown(); len(shown) > 0 && !slices.Contains(shown, m.selected) {
		m.selected = shown[0]
	}
}

// move steps the highlight through the rows the filter shows.
func (m *Model) move(delta int) {
	shown := m.shown()
	if at := slices.Index(shown, m.selected) + delta; at >= 0 && at < len(shown) {
		m.selected = shown[at]
	}
}

// handleForkKey takes the keys of the filter: / starts it, and while it
// is being typed every key but enter belongs to it. tab hands the keys
// back to the list with the filter kept, so a and space can mark what
// it shows; esc drops the filter before a second esc closes the picker.
// It also takes the moves, which skip the rows a filter hides, and
// enter when no row is shown.
func (m *Model) handleForkKey(key string) bool {
	if key == "enter" {
		return len(m.shown()) == 0
	}
	if m.filtering {
		switch key {
		case "esc":
			m.filtering = false
			m.setFilter("")
		case "tab":
			m.filtering = false
		case "down", "ctrl+n":
			m.move(1)
		case "up", "ctrl+p":
			m.move(-1)
		case "backspace":
			if r := []rune(m.filter); len(r) > 0 {
				m.setFilter(string(r[:len(r)-1]))
			}
		case "space":
			m.setFilter(m.filter + " ")
		default:
			if utf8.RuneCountInString(key) == 1 {
				m.setFilter(m.filter + key)
			}
		}
		return true
	}
	switch key {
	case "/":
		m.filtering = true
	case "esc":
		if m.filter == "" {
			return false
		}
		m.setFilter("")
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	default:
		return false
	}
	return true
}
