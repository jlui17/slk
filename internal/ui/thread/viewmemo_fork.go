package thread

import (
	"io"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/entrylist"
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
	counted        int // the count on the replies line
	parentGen      uint64
	boundary       string
	today          string
}

// parentRow returns the parent's entry for this frame, rendering it only
// when its key changed or the cache was dropped. Its image uploads go
// out while it is on screen (flushVisible).
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
	return m.memo.parent
}

// renderParentRow renders the parent as entry 0 of the list. The
// parent's image uploads are returned for the memo to keep;
// flushVisible sends them while the parent is on screen. We
// discard the parent's reaction hits in v1, as before; its image hits
// are kept so a click on its image opens the preview.
func (m *Model) renderParentRow(width int, parentIsSelected bool) (entrylist.Entry, []func(io.Writer) error) {
	// Replies only get a day divider when their day differs from the
	// parent's, so the parent header carries its own date; otherwise a
	// thread whose first replies share the parent's day shows no date.
	// Not for a summary-built parent that has no time yet.
	opt := threadOptions(parentIsSelected)
	if date := messages.DateFromTS(m.parent.TS); date != "" && m.parent.Timestamp != "" {
		opt.HeaderDate = messages.FormatShortDate(date)
	}
	// The header names the author: a parent built without a name (from
	// a summary) takes the one the breadcrumb used to show.
	named := m.parent
	named.UserName = m.breadcrumbAuthor()
	parentRendered := messages.RenderMessage(named, width, m.renderContext(), opt)
	if parentIsSelected && m.nav.Active {
		m.marks.navDrawn = append(m.marks.navDrawn, m.parent.TS)
	}
	parentContent := parentRendered.Content
	flushes := slices.Concat(parentRendered.BodyFlushes, parentRendered.BlockFlushes, parentRendered.ReactionFlushes, parentRendered.AttachmentFlushes)
	m.parentEntry = messages.EntryPayload{
		CodeBlockCopyLabels: parentRendered.CodeBlockCopyLabels,
		AttachmentFoldRows:  parentRendered.AttachmentFoldRows,
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
			parentContent = messages.SelectedContent(named, parentRendered)
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
	parent := entrylist.Entry{
		ID:         m.parent.TS,
		Ref:        parentSelected,
		Selectable: true,
		Lines:      strings.Split(parentContent, "\n"),
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

// setReplyList hands the list its entries: the parent, a blank row and
// the replies line under it ("N replies ───", as Slack's thread view
// has it; none while no reply counts), then for each reply the blank
// rows above it, its day divider (when its local day
// differs from the previous one, seeded with the parent's), the
// new-messages line before the first reply past the unread boundary,
// and the reply. One blank row separates replies from the same person,
// two a change of speaker; a divider has one above and one below.
// Dividers and blank rows are decorations: not selectable, not
// text-selectable. When nothing they are built from changed, the list
// keeps the entries it has.
func (m *Model) setReplyList(width int, keepPosition bool, parent entrylist.Entry) {
	key := listKey{
		width:     width,
		replies:   len(m.replyEntries),
		counted:   m.visibleReplyCount(),
		parentGen: m.memo.parentGen,
		boundary:  m.unreadBoundaryTS,
		today:     messages.Today(),
	}
	if m.memo.listOK && m.memo.listAt == key {
		return
	}
	m.memo.listAt, m.memo.listOK = key, true
	m.regroupReplies(width) // fork: grouping_fork.go

	decoration := func(rows ...string) entrylist.Entry { return entrylist.Entry{Ref: -1, Lines: rows} }
	entries := make([]entrylist.Entry, 0, 2*len(m.replyEntries)+2)
	entries = append(entries, parent)
	if key.counted > 0 {
		entries = append(entries, decoration(messages.BlankRows(width, 1)[0], messages.RepliesLine(width, key.counted)))
	}
	lastDate := messages.DateFromTS(m.parent.TS)
	for i, e := range m.replyEntries {
		reply := m.replies[i]
		replyDate := messages.DateFromTS(reply.TS)
		newDay := replyDate != "" && replyDate != lastDate
		newLine := m.newLineBefore(i)
		gap := 1
		if i > 0 && !newDay && !newLine {
			gap = messages.SpacerRows(m.replies[i-1], reply)
		}
		entries = append(entries, decoration(messages.BlankRows(width, gap)...))
		if newDay {
			entries = append(entries, decoration(strings.Split(messages.DayDivider(width, messages.FormatDateSeparator(replyDate)), "\n")...))
			lastDate = replyDate
		}
		if newLine {
			entries = append(entries, decoration(strings.Split(messages.NewMessagesDivider(width), "\n")...))
		}
		entries = append(entries, e)
	}
	m.list.SetEntries(entries, width, keepPosition)
}

// ShowsMessage reports whether ts is the parent's or a reply's. The App
// asks it when an image lands: the parent's entry is kept between
// frames, so an image of the parent must invalidate as a reply's does.
func (m *Model) ShowsMessage(ts string) bool {
	return ts != "" && ts == m.parent.TS || m.HasReply(ts)
}
