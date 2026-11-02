// Package pinsbookmarks is the dialog that lists a conversation's pins
// and its bookmarks (the p and B keys): one centered box in the link
// picker's frame, a Pins section and then a Bookmarks section, moved
// through as one list. The App fetches the lists through Service, turns
// them into rows (PinRow, BookmarkRow) and opens what HandleKey chooses.
package pinsbookmarks

import (
	"context"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/chrome"
	"github.com/gammons/slk/internal/text"
)

// Pin is one pinned message or file, as Slack lists it.
type Pin struct {
	// TS and ThreadTS are the pinned message's; both are empty for a
	// pinned file.
	TS, ThreadTS string
	// UserID is the message's author; Username is the name a bot posts
	// under, for a message without a user.
	UserID, Username string
	// Text is the message's raw mrkdwn.
	Text string
	// FileName is the pinned file's name, or the first file's name of
	// a pinned message.
	FileName  string
	Permalink string
}

// Bookmark is one bookmarked link.
type Bookmark struct {
	Title, Link string
	// Folder is the title of the folder the link is in, "" for none.
	Folder string
}

// Service reads a conversation's pins, the most recently pinned first,
// its bookmarks, in Slack's rank order with each folder's links in its
// place, and how many pins it has, without reading them. Slack limiting
// requests is a *core.RateLimitedError.
type Service interface {
	Pins(ctx context.Context, teamID, channelID string) ([]Pin, error)
	Bookmarks(ctx context.Context, teamID, channelID string) ([]Bookmark, error)
	PinCount(ctx context.Context, teamID, channelID string) (int, error)
}

// PinRow is a pin as the dialog draws it.
type PinRow struct {
	Author, Day string
	// Text is the message as one line of plain text, or the file's
	// name when File is set.
	Text     string
	File     bool
	InThread bool
}

// BookmarkRow is a bookmark as the dialog draws it.
type BookmarkRow struct {
	Title, Site, Folder string
	// InApp marks a link slk opens itself, drawn with the link
	// picker's [slk] badge.
	InApp bool
}

// Section is one of the dialog's two lists.
type Section int

const (
	Pins Section = iota
	Bookmarks
)

// Kind is what HandleKey asks the App to do.
type Kind int

const (
	None Kind = iota
	// Open opens the chosen item in place; OpenTab and OpenPane open
	// it in a new herdr tab or pane, as O and alt+o do for a link.
	Open
	OpenTab
	OpenPane
	// Retry loads again the lists that failed to load.
	Retry
)

// Action is HandleKey's answer: Kind, and for an open, the item.
type Action struct {
	Kind    Kind
	Section Section
	Index   int
}

// list is what the dialog knows of one section's list besides its rows.
type list struct {
	loading bool
	// err says why the list did not load, "" when it did.
	err string
	// folded is each row's text as the filter reads it; shown is the
	// rows the filter keeps.
	folded []string
	shown  []int
}

// pinColumns is the widths of the pins' author and day columns, gap
// included, and whether any pin is a thread reply, which reserves the
// column that says so.
type pinColumns struct {
	author, day int
	thread      bool
}

// Model is the dialog's state.
type Model struct {
	visible bool
	// title is the conversation as the dialog names it, "#general".
	title     string
	pins      []PinRow
	bookmarks []BookmarkRow
	lists     [2]list
	pinCols   pinColumns

	section   Section
	selected  [2]int // per section, an index into its list
	filter    string
	filtering bool
	pendingG  bool

	// laid is the last layout, kept until the next change, so a click
	// lays the dialog out once for BoxSize and ClickRow.
	laid *layout
}

var _ interface {
	chrome.Dialog
	chrome.Clickable
} = (*Model)(nil)

// Open shows the dialog for conversation title, on section, both lists
// loading until SetError, SetPins and SetBookmarks fill them.
func (m *Model) Open(title string, section Section) {
	*m = Model{visible: true, title: title, section: section, lists: [2]list{{loading: true}, {loading: true}}}
}

// Close hides the dialog.
func (m *Model) Close() { m.visible = false }

// IsVisible reports whether the dialog is showing.
func (m *Model) IsVisible() bool { return m.visible }

// Section is the section the cursor is in.
func (m *Model) Section() Section { return m.section }

// String is the status bar's mode for the section the cursor is in.
func (m *Model) String() string {
	if m.section == Bookmarks {
		return "BOOKMARKS"
	}
	return "PINS"
}

// SetLoading shows that section s is loading.
func (m *Model) SetLoading(s Section) {
	m.laid = nil
	m.lists[s] = list{loading: true}
	m.fixSelection()
}

// SetError shows that section s failed to load: text says why.
func (m *Model) SetError(s Section, text string) {
	m.laid = nil
	m.lists[s] = list{err: text}
	m.fixSelection()
}

// SetPins shows the pins, keeping the cursor on the row it was on when
// it is still there.
func (m *Model) SetPins(rows []PinRow) {
	m.pins = rows
	m.pinCols = pinColumns{}
	folded := make([]string, len(rows))
	for i, p := range rows {
		folded[i] = text.Fold(p.Author + " " + p.Text)
		m.pinCols.author = max(m.pinCols.author, min(lipgloss.Width(p.Author), 16))
		m.pinCols.day = max(m.pinCols.day, lipgloss.Width(p.Day))
		m.pinCols.thread = m.pinCols.thread || p.InThread
	}
	m.pinCols.author += 2
	m.pinCols.day += 2
	m.setRows(Pins, folded)
}

// SetBookmarks shows the bookmarks, as SetPins the pins.
func (m *Model) SetBookmarks(rows []BookmarkRow) {
	m.bookmarks = rows
	folded := make([]string, len(rows))
	for i, b := range rows {
		folded[i] = text.Fold(b.Title + " " + b.Site + " " + b.Folder)
	}
	m.setRows(Bookmarks, folded)
}

func (m *Model) setRows(s Section, folded []string) {
	m.laid = nil
	m.lists[s] = list{folded: folded}
	m.refilter(s)
	m.fixSelection()
}

// HandleKey takes one key, by its tea name ("j", "enter", "alt+o").
func (m *Model) HandleKey(key string) Action {
	m.laid = nil
	if m.filtering {
		return m.filterKey(key)
	}
	pendingG := m.pendingG
	m.pendingG = false
	switch key {
	case "g":
		if !pendingG {
			m.pendingG = true
		} else if shown := m.lists[m.section].shown; len(shown) > 0 {
			m.selected[m.section] = shown[0]
		}
	case "G":
		if shown := m.lists[m.section].shown; len(shown) > 0 {
			m.selected[m.section] = shown[len(shown)-1]
		}
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "tab", "shift+tab":
		m.switchTo(m.other())
	case "enter", "o":
		return m.choose(Open)
	case "O":
		return m.choose(OpenTab)
	case "alt+o":
		return m.choose(OpenPane)
	case "/":
		m.filtering = m.hasRows(Pins) || m.hasRows(Bookmarks)
	case "esc":
		if m.filter != "" {
			m.setFilter("")
		} else {
			m.Close()
		}
	case "q":
		m.Close()
	case "r":
		if m.lists[Pins].err != "" || m.lists[Bookmarks].err != "" {
			return Action{Kind: Retry}
		}
	case "p":
		m.toggle(Pins)
	case "B":
		m.toggle(Bookmarks)
	}
	return Action{}
}

// filterKey takes the keys while the filter is typed: esc drops it,
// tab hands the keys back to the list with the filter kept, enter
// opens, the arrows move, and the rest edits it.
func (m *Model) filterKey(key string) Action {
	switch key {
	case "esc":
		m.filtering = false
		m.setFilter("")
	case "tab":
		m.filtering = false
	case "enter":
		m.filtering = false
		return m.choose(Open)
	case "down", "ctrl+n":
		m.move(1)
	case "up", "ctrl+p":
		m.move(-1)
	default:
		if filter, ok := chrome.EditFilter(m.filter, key); ok {
			m.setFilter(filter)
		}
	}
	return Action{}
}

// toggle is p and B: on their own section they close the dialog, on
// the other they move the cursor to theirs.
func (m *Model) toggle(s Section) {
	if m.section == s {
		m.Close()
		return
	}
	m.switchTo(s)
}

// switchTo moves the cursor to section s, when the filter shows any of
// it, keeping the row it had there.
func (m *Model) switchTo(s Section) {
	shown := m.lists[s].shown
	if len(shown) == 0 {
		return
	}
	m.section = s
	if !slices.Contains(shown, m.selected[s]) {
		m.selected[s] = shown[0]
	}
}

func (m *Model) other() Section {
	if m.section == Pins {
		return Bookmarks
	}
	return Pins
}

// move steps the cursor through the rows the filter shows, from the
// last pin on to the first bookmark and back: the two sections are one
// list to j and k.
func (m *Model) move(delta int) {
	shown := m.lists[m.section].shown
	at := slices.Index(shown, m.selected[m.section]) + delta
	switch {
	case at >= 0 && at < len(shown):
		m.selected[m.section] = shown[at]
	case delta > 0 && m.section == Pins:
		if next := m.lists[Bookmarks].shown; len(next) > 0 {
			m.section, m.selected[Bookmarks] = Bookmarks, next[0]
		}
	case delta < 0 && m.section == Bookmarks:
		if prev := m.lists[Pins].shown; len(prev) > 0 {
			m.section, m.selected[Pins] = Pins, prev[len(prev)-1]
		}
	}
}

// choose closes the dialog on the cursor's row and asks for it to be
// opened. Nothing happens without a row under the cursor.
func (m *Model) choose(kind Kind) Action {
	if !slices.Contains(m.lists[m.section].shown, m.selected[m.section]) {
		return Action{}
	}
	m.Close()
	return Action{Kind: kind, Section: m.section, Index: m.selected[m.section]}
}

func (m *Model) setFilter(filter string) {
	m.filter = filter
	m.refilter(Pins)
	m.refilter(Bookmarks)
	m.fixSelection()
}

// refilter recomputes which of section s's rows the filter shows: those
// whose text holds it, whatever the case and accents.
func (m *Model) refilter(s Section) {
	l := &m.lists[s]
	want := text.Fold(m.filter)
	l.shown = l.shown[:0]
	for i, f := range l.folded {
		if strings.Contains(f, want) {
			l.shown = append(l.shown, i)
		}
	}
}

// fixSelection puts each section's cursor on a row the filter shows,
// and the cursor in the other section when its own, loaded, shows none.
func (m *Model) fixSelection() {
	for s := range m.lists {
		if shown := m.lists[s].shown; len(shown) > 0 && !slices.Contains(shown, m.selected[s]) {
			m.selected[s] = shown[0]
		}
	}
	own := m.lists[m.section]
	if !own.loading && len(own.shown) == 0 && len(m.lists[m.other()].shown) > 0 {
		m.section = m.other()
	}
}

// hasRows reports whether section s has loaded rows.
func (m *Model) hasRows(s Section) bool { return len(m.lists[s].folded) > 0 }
