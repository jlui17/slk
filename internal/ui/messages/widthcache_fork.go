package messages

import "github.com/gammons/slk/internal/bubbles/entrylist"

// widthSlot is the render cache of the width the pane was drawn at
// before the current one. Opening and closing a thread switches the
// channel pane between two widths; with the other width's entries kept,
// each switch after the first restores them instead of rendering every
// message again. One slot: the pane keeps at most two widths.
//
// The slot holds entries of the messages the pane shows, one for one, so
// a restore carries the scroll position over by entry. Any change to the
// message list drops it (another channel, an older page, a new or
// deleted message, the "new" divider moving), as does whatever drops
// the current cache: swapWidth sees a relayout or a nil cache. A change
// to one message while it is put away it carries as a stale mark and
// renders when it is restored: every mark the current cache consumes is
// copied to the slot (note).
type widthSlot struct {
	ok      bool
	width   int
	entries []entrylist.Entry
	cache   []EntryPayload
	focused bool // cacheFocused of the build that rendered it

	stale map[string]struct{}
}

// note records the entries of stale for the slot to render again.
func (s *widthSlot) note(stale map[string]struct{}) {
	if !s.ok {
		return
	}
	if len(stale) > 0 && s.stale == nil {
		s.stale = make(map[string]struct{}, len(stale))
	}
	for ts := range stale {
		s.stale[ts] = struct{}{}
	}
}

// swapWidth runs before buildCache. On a width change it puts the
// current cache away and, when the slot holds the new width, restores
// it, reporting true when nothing is left for buildCache to do. In any
// other case it reports false and buildCache builds as before.
func (m *Model) swapWidth(width int, keepPosition bool) bool {
	// Every build at the current width is one of these.
	if m.cache == nil || m.relayout || m.cacheMsgLen != len(m.messages) || m.cacheWidth == width {
		m.alt = widthSlot{} // the messages changed: no width's entries are theirs
		return false
	}
	cur := widthSlot{ok: true, width: m.cacheWidth, entries: m.list.Entries(), cache: m.cache, focused: m.cacheFocused}
	cur.note(m.staleEntries)
	m.alt.note(m.staleEntries)
	alt := m.alt
	m.alt = cur
	if !alt.ok || alt.width != width {
		m.cache = nil // the build must not reuse cur.cache's array
		return false
	}
	m.cache, m.cacheWidth, m.cacheFocused = alt.cache, width, alt.focused
	m.list.SetEntries(alt.entries, width, keepPosition) // the same messages as cur's
	m.buildCacheStyles(width)                           // the spacer and "more below" rows of this width
	m.staleEntries = alt.stale
	if len(m.staleEntries) > 0 {
		m.alt = widthSlot{} // cur has applied these marks: none to copy
		m.partialRebuild(width)
		m.alt = cur
	}
	return true
}
