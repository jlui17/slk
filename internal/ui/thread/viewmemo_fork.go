package thread

import (
	"io"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// viewMemo keeps the parts of a thread frame that only change with the
// thread: the parent's entry and the list of entries. A frame with
// nothing changed (j/k, a focus move, a reply in another thread) reuses
// both, and the list keeps its scrollbar cache, which a SetEntries
// drops. dropCache zeroes it, so every invalidation of the replies
// renders the parent again too: they read the same inputs.
type viewMemo struct {
	parentOK bool
	parentAt parentKey
	parent   entrylist.Entry
	flushes  []func(io.Writer) error
	// parentGen counts parent renders, so the list knows which parent
	// entry it holds.
	parentGen uint64

	listOK bool
	listAt listKey
}

// parentKey is what the parent's entry depends on beyond the inputs
// whose changes invalidate the cache.
type parentKey struct {
	width             int
	selected, focused bool
	shortHeader       bool   // selected while a text selection exists
	today             string // the day "Today" and its kin are relative to
}

// listKey is what the list of entries depends on beyond the reply
// cache, which dropCache invalidates.
type listKey struct {
	width, replies int
	parentGen      uint64
	boundary       string
	today          string
}

// parentRow returns the parent's entry for this frame, rendering it only
// when its key changed or the cache was dropped. Its image uploads go
// out every frame, as when it was rendered every frame.
func (m *Model) parentRow(width int) entrylist.Entry {
	parentIsSelected := m.selected == parentSelected
	key := parentKey{
		width:       width,
		selected:    parentIsSelected,
		focused:     m.focused,
		shortHeader: parentIsSelected && m.list.HasSelection(),
		today:       messages.Today(),
	}
	if !m.memo.parentOK || m.memo.parentAt != key {
		m.memo.parent, m.memo.flushes = m.renderParentRow(width, parentIsSelected)
		m.memo.parentAt, m.memo.parentOK = key, true
		m.memo.parentGen++
	}
	for _, fl := range m.memo.flushes {
		if fl != nil {
			_ = fl(imgpkg.KittyOutput)
		}
	}
	return m.memo.parent
}

// renderParentRow renders the parent as entry 0 of the list: its rows,
// then the rule under it. The rule is not selectable text (Plain stops
// before it). The parent's image uploads go out with it rather than
// with the replies'; they are returned for parentRow to send. We
// discard the parent's reaction hits in v1, as before; its image hits
// are kept so a click on its image opens the preview.
func (m *Model) renderParentRow(width int, parentIsSelected bool) (entrylist.Entry, []func(io.Writer) error) {
	parentRendered := m.renderMessage(m.parent, width, parentIsSelected)
	if parentIsSelected && m.nav.Active {
		m.marks.navDrawn = append(m.marks.navDrawn, m.parent.TS)
	}
	parentContent, parentHeader, parentHeaderBudget := parentRendered.Content, parentRendered.Header, parentRendered.HeaderBudget
	flushes := slices.Concat(parentRendered.BodyFlushes, parentRendered.BlockFlushes, parentRendered.ReactionFlushes, parentRendered.AttachmentFlushes)
	m.parentEntry = messages.EntryPayload{
		CodeBlockCopyLabels: parentRendered.CodeBlockCopyLabels,
		AttachmentFoldRows:  parentRendered.AttachmentFoldRows,
	}
	// Replies only get a day divider when their day differs from the
	// parent's, so the parent header carries its own date; otherwise a
	// thread whose first replies share the parent's day shows no date.
	// Before the plain mirror, so copy and drag highlight match what is
	// drawn. Skipped when the header would not fit, and for a
	// summary-built parent that has no time yet.
	if !parentIsSelected && m.parent.Timestamp != "" {
		if date := messages.DateFromTS(m.parent.TS); date != "" {
			dated := messages.FormatShortDate(date) + ", " + m.parent.Timestamp
			parentContent = messages.ReplaceHeaderTimestamp(parentContent, parentHeader, m.parent.Timestamp, dated, parentHeaderBudget)
		}
	}
	parentPlain := messages.PlainLines(parentContent)
	// Selection border for the parent row mirrors the per-reply
	// cache-build treatment (borderSelect / borderInvis below): thick
	// left border + tint when the cursor sits on the parent, invisible
	// border otherwise so the parent's width matches the reply rows.
	if parentIsSelected {
		// After the plain mirror, so it keeps the short form. Skipped
		// while a text selection exists; see selectedLines.
		if !m.list.HasSelection() {
			parentContent = messages.SelectedHeader(parentContent, parentHeader, m.parent.TS, m.parent.Timestamp, parentHeaderBudget)
		}
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.SelectionBorderColor(m.focused)).
			BorderBackground(styles.SelectionTintColor(m.focused)).
			Background(styles.SelectionTintColor(m.focused)).
			Render(parentContent)
	} else {
		parentContent = lipgloss.NewStyle().BorderStyle(thickLeftBorder).BorderLeft(true).
			BorderForeground(styles.Background).BorderBackground(styles.Background).
			Render(parentContent)
	}
	// Pad every parent row to the pane width with the theme background.
	// The outer wrapper used to do this (the parent, unlike a reply, is
	// not width-filled when rendered); this is the same per-row padding,
	// applied where it is needed.
	parentContent = lipgloss.NewStyle().Width(width).Background(styles.Background).Render(parentContent)
	parentSeparator := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border).
		Render(strings.Repeat("─", width))
	parent := entrylist.Entry{
		ID:         m.parent.TS,
		Ref:        parentSelected,
		Selectable: true,
		Lines:      append(strings.Split(parentContent, "\n"), parentSeparator),
		ColOffset:  1,
	}
	if m.parent.TS != "" {
		// A parent without a ts cannot be clicked or text-selected.
		parent.Plain = parentPlain
		for _, h := range parentRendered.Hits {
			if h.Kind == entrylist.HitImage || h.Kind == entrylist.HitFile {
				parent.Hits = append(parent.Hits, h)
			}
		}
	}
	return parent, flushes
}

// setReplyList hands the list its entries: the parent, then for each
// reply its day divider (when its local day differs from the previous
// one, seeded with the parent's), the "── new ──" landmark before the
// first reply past the unread boundary, the reply, and a full-width
// rule between replies. Dividers, landmark and rules are decorations:
// not selectable, not text-selectable. When nothing they are built from
// changed, the list keeps the entries it has.
func (m *Model) setReplyList(width int, keepPosition bool, parent entrylist.Entry) {
	key := listKey{
		width:     width,
		replies:   len(m.replyEntries),
		parentGen: m.memo.parentGen,
		boundary:  m.unreadBoundaryTS,
		today:     messages.Today(),
	}
	if m.memo.listOK && m.memo.listAt == key {
		return
	}
	m.memo.listAt, m.memo.listOK = key, true

	ruleStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.Border)
	replySeparator := ruleStyle.Render(strings.Repeat("─", width))
	var newLandmark string
	if m.unreadBoundaryTS != "" {
		newLandmark = lipgloss.NewStyle().
			Width(width).
			Background(styles.Background).
			Foreground(styles.Error).
			Bold(true).
			Align(lipgloss.Center).
			Render("── new ──")
	}
	dateSeparatorStyle := lipgloss.NewStyle().
		Width(width).
		Background(styles.Background).
		Foreground(styles.TextMuted).
		Bold(true).
		Align(lipgloss.Center)
	decoration := func(row string) entrylist.Entry { return entrylist.Entry{Ref: -1, Lines: []string{row}} }
	entries := make([]entrylist.Entry, 0, 2*len(m.replyEntries)+2)
	entries = append(entries, parent)
	lastDate := messages.DateFromTS(m.parent.TS)
	landmarkInserted := false
	for i, e := range m.replyEntries {
		if replyDate := messages.DateFromTS(m.replies[i].TS); replyDate != "" && replyDate != lastDate {
			entries = append(entries, decoration(dateSeparatorStyle.Render("── "+messages.FormatDateSeparator(replyDate)+" ──")))
			lastDate = replyDate
		}
		if !landmarkInserted && newLandmark != "" && m.replies[i].TS > m.unreadBoundaryTS {
			entries = append(entries, decoration(newLandmark))
			landmarkInserted = true
		}
		entries = append(entries, e)
		if i < len(m.replyEntries)-1 {
			entries = append(entries, decoration(replySeparator))
		}
	}
	m.list.SetEntries(entries, width, keepPosition)
}

// ShowsMessage reports whether ts is the parent's or a reply's. The App
// asks it when an image lands: the parent's entry is kept between
// frames, so an image of the parent must invalidate as a reply's does.
func (m *Model) ShowsMessage(ts string) bool {
	return ts != "" && ts == m.parent.TS || m.HasReply(ts)
}
