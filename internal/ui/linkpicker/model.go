// Package linkpicker provides the modal overlay that lets the user
// pick one item from a message: which link to open (the `o`
// keybinding) or which file attachment to download (the `d`
// keybinding). The chosen item is dispatched as ui.OpenLinkMsg or
// ui.DownloadFileMsg by the mode handler, depending on the kind the
// App recorded when opening the picker. The `O` picker that opens
// herdr tabs also lets the user mark several rows (SetMultiSelect);
// the mode handler dispatches those as ui.OpenLinksInHerdrTabsMsg.
package linkpicker

// Item is one selectable row.
type Item struct {
	URL   string
	Label string // filename for file rows; link label (may be empty) for links
	// Display replaces URL in the rendered row when non-empty (a
	// decoded permalink description, later the fetched message
	// snippet). URL stays the open target either way.
	Display string
	// Detail is trailing muted info right after the text: the file size
	// for file rows, the line count for code-block rows.
	Detail string
	// InApp marks links that the router will navigate inside slk
	// (active-workspace archive permalinks); rendered with a badge.
	InApp bool
	// Fork: Side is muted text in an aligned column of its own, right
	// of the row's text: where a permalink points and, once its preview
	// has landed, who wrote it. See view_fork.go.
	Side string
	// Fork: previewed is set once SetDisplay has filled the row.
	previewed bool
	// Fork: FilterText is all the filter reads of the row. Open sets
	// it to the row's text when the opener left it empty.
	FilterText string
	// Index is the item's position in the slice passed to Open,
	// assigned by Open so the dispatcher can map the chosen row back
	// to its source data.
	Index int
}

// Model is the picker overlay state.
type Model struct {
	title    string
	items    []Item
	selected int
	visible  bool

	// Fork: row marking, the filter and the scroll window, see
	// model_fork.go.
	multiSelect bool
	marked      map[int]bool
	filter      string
	filtering   bool
	termHeight  int
	top         int
	rowWidth    int
}

// New creates a hidden picker.
func New() *Model { return &Model{} }

// Open shows the picker over items with the given dialog title, first
// row selected.
func (m *Model) Open(title string, items []Item) {
	m.title = title
	m.items = items
	for i := range m.items {
		m.items[i].Index = i
	}
	m.selected = 0
	m.visible = true
	m.resetFork()
}

// Close hides the picker and drops its items.
func (m *Model) Close() {
	m.visible = false
	m.items = nil
	m.selected = 0
	m.resetFork()
}

// IsVisible reports whether the picker is showing.
func (m *Model) IsVisible() bool { return m.visible }

// Title returns the dialog title set by Open.
func (m *Model) Title() string { return m.title }

// Items returns the current rows (for rendering and tests).
func (m *Model) Items() []Item { return m.items }

// Selected returns the highlighted row index.
func (m *Model) Selected() int { return m.selected }

// HandleKey processes one key. Returns (item, true) when the user
// chose a row with enter (the picker closes itself); (Item{}, false)
// otherwise. esc/q close without choosing.
func (m *Model) HandleKey(key string) (Item, bool) {
	if m.handleForkKey(key) {
		return Item{}, false
	}
	switch key {
	case "esc", "q":
		m.Close()
	case "j", "down":
		if m.selected < len(m.items)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "enter":
		if len(m.items) == 0 {
			return Item{}, false
		}
		item := m.items[m.selected]
		m.Close()
		return item, true
	}
	return Item{}, false
}
