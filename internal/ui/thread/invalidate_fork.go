package thread

import (
	"math"
	"reflect"
	"slices"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/messages"
)

// The thread pane's targeted invalidations, as the channel pane's
// (messages/invalidate_fork.go): an event re-renders only the replies
// whose rows it changes, and the parent only when it is one of them.
// Each still clears the layout and the chrome, as InvalidateCache does.
// invalidate_oracle_fork_test.go holds every one of them to a full
// rebuild, byte for byte.

// replyMarks is what changed in the reply cache since the last full
// build.
type replyMarks struct {
	// stale holds the ts of each reply to render again.
	stale map[string]struct{}
	// relayout reports replies added, removed or replaced by another
	// ts: the reply cache is rebuilt around the replies it keeps.
	relayout bool
	// builtFocused is m.focused at the last full build: every reply's
	// selected rows carry its colours, so a rebuild of some replies
	// under another focus builds them all.
	builtFocused bool
	// navDrawn holds the ts of each message rendered as the cursor's
	// while reaction nav was active: it carries the highlight until it
	// is rendered again.
	navDrawn []string
}

// begin resets the marks for a full build under focused. navDrawn
// stays: the parent, which a full build of the replies leaves alone,
// may be in it.
func (r *replyMarks) begin(focused bool) {
	r.stale, r.relayout, r.builtFocused = nil, false, focused
}

// invalidateLayout is InvalidateCache without dropping the reply cache:
// the layout and the chrome are built again.
func (m *Model) invalidateLayout() {
	m.layoutValid = false
	m.chromeCacheValid = false
	m.dirty()
}

// invalidateReplies has the next View render the messages with these
// ts again, the parent's or replies'.
func (m *Model) invalidateReplies(ts ...string) {
	for _, t := range ts {
		m.markStale(t)
	}
	m.invalidateLayout()
}

// relayoutReplies has the next View rebuild the reply cache after
// replies were added, removed or replaced, keeping the entry of every
// reply that is still there and not stale. Until then lookups see only
// the parent, as after dropCache.
func (m *Model) relayoutReplies() {
	if m.cache != nil {
		m.marks.relayout = true
		m.list.KeepLookups(1)
	}
	m.invalidateLayout()
}

// invalidateNav redraws what a reaction-nav change changes: the
// cursor's message and every message drawn with the highlight since the
// last change.
func (m *Model) invalidateNav() {
	for _, ts := range m.marks.navDrawn {
		m.markStale(ts)
	}
	m.marks.navDrawn = m.marks.navDrawn[:0]
	if msg := m.SelectedReply(); msg != nil {
		m.markStale(msg.TS)
	}
	m.invalidateLayout()
}

// invalidateAuthorsOf is invalidateAuthors for one user's status: only
// the messages they wrote render again.
func (m *Model) invalidateAuthorsOf(userID string) {
	m.userNamesV++
	if m.parent.UserID == userID {
		m.markStale(m.parent.TS)
	}
	for i := range m.replies {
		if m.replies[i].UserID == userID {
			m.markStale(m.replies[i].TS)
		}
	}
	m.layoutValid = false
}

// markUserStale marks the messages a change of userID's name changes:
// those they wrote and those that mention them (messages.RefersTo).
func (m *Model) markUserStale(userID string) {
	if m.parent.UserID == userID || messages.RefersTo(&m.parent, userID) {
		m.markStale(m.parent.TS)
	}
	for i := range m.replies {
		if r := &m.replies[i]; r.UserID == userID || messages.RefersTo(r, userID) {
			m.markStale(r.TS)
		}
	}
}

// markStale has the next View render the message with ts again: the
// parent (its entry is kept in m.memo) or a reply. With no reply cache,
// or no ts to find the entry by, the next View builds it whole.
func (m *Model) markStale(ts string) {
	if ts == "" { // no entry to find
		m.dropCache()
		return
	}
	if ts == m.parent.TS {
		m.memo.parentOK = false
		return
	}
	if m.cache == nil {
		return
	}
	if m.marks.stale == nil {
		m.marks.stale = make(map[string]struct{})
	}
	m.marks.stale[ts] = struct{}{}
}

// reconcileReplies prepares SetThread to keep what it can of the reply
// cache: for a reload of the open thread, the entry of every reply that
// comes back unchanged (same ts, same content), and the parent's when
// it is unchanged. Any other thread drops the cache.
func (m *Model) reconcileReplies(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	if m.cache == nil || channelID != m.channelID || threadTS != m.threadTS {
		m.dropCache()
		return
	}
	if !reflect.DeepEqual(parent, m.parent) {
		m.memo.parentOK = false
	}
	old := make(map[string]*messages.MessageItem, len(m.replies))
	for i := range m.replies {
		old[m.replies[i].TS] = &m.replies[i]
	}
	if _, ok := old[""]; ok || slices.ContainsFunc(replies, func(r messages.MessageItem) bool { return r.TS == "" }) {
		m.dropCache()
		return
	}
	for i := range replies {
		if prev, ok := old[replies[i].TS]; ok && !reflect.DeepEqual(*prev, replies[i]) {
			m.markStale(replies[i].TS)
		}
	}
	m.relayoutReplies()
}

// refreshStaleReplies brings the reply cache up to date with the marks
// before View's full-build check, rendering only the stale and new
// replies. It leaves a full build to that check when one is due: no
// cache, another width, a focus other than the last build's (see
// replyMarks.builtFocused), or replies moved while reaction nav is
// active (the cursor's reply draws it).
func (m *Model) refreshStaleReplies(width int) {
	marks := &m.marks
	if m.cache == nil || m.cacheWidth != width || !marks.relayout && len(marks.stale) == 0 {
		return
	}
	if !marks.relayout && m.cacheReplyLen != len(m.replies) {
		return
	}
	if m.focused != marks.builtFocused || marks.relayout && m.nav.Active {
		m.dropCache()
		return
	}
	wrap := messages.NewWrapStyles(m.focused)
	if marks.relayout {
		entries := make([]entrylist.Entry, 0, len(m.replies))
		cache := make([]messages.EntryPayload, 0, len(m.replies))
		index := make(map[string]int, len(m.replies))
		for i, reply := range m.replies {
			j, kept := m.replyIDToIdx[reply.TS]
			if _, stale := marks.stale[reply.TS]; stale || !kept || j >= len(m.replyEntries) {
				e, p := m.renderReplyEntry(i, reply, width, wrap)
				entries, cache = append(entries, e), append(cache, p)
			} else {
				e := m.replyEntries[j]
				e.Ref = i
				entries, cache = append(entries, e), append(cache, m.cache[j])
			}
			index[reply.TS] = i
		}
		m.replyEntries, m.cache, m.replyIDToIdx = entries, cache, index
		m.cacheReplyLen = len(m.replies)
	} else {
		for ts := range marks.stale {
			if i, ok := m.replyIDToIdx[ts]; ok && i < len(m.replies) {
				m.replyEntries[i], m.cache[i] = m.renderReplyEntry(i, m.replies[i], width, wrap)
			}
		}
	}
	marks.stale, marks.relayout = nil, false
	m.memo.listOK = false
	m.flushKey.selected = math.MinInt // re-emit the kitty uploads
}

// renderReplyEntry renders reply i: its list entry and its payload.
func (m *Model) renderReplyEntry(i int, reply messages.MessageItem, width int, wrap messages.WrapStyles) (entrylist.Entry, messages.EntryPayload) {
	// renderMessage's last arg ("isSelected") drives reaction-
	// nav pill highlighting, so it MUST be i == m.selected.
	r := m.renderMessage(reply, width, i == m.selected)
	if i == m.selected && m.nav.Active {
		m.marks.navDrawn = append(m.marks.navDrawn, reply.TS)
	}
	// The thread pane's kitty upload order: body, blocks,
	// reactions, then attachments.
	flushes := slices.Concat(r.BodyFlushes, r.BlockFlushes, r.ReactionFlushes, r.AttachmentFlushes)
	w := messages.WrapRows(reply, r, width, wrap)
	return entrylist.Entry{
			ID:         reply.TS,
			Ref:        i,
			Selectable: true,
			Lines:      w.Lines,
			Selected:   w.Selected,
			Plain:      w.Plain,
			ColOffset:  1,
			Hits:       r.Hits,
		}, messages.EntryPayload{ShortSrc: w.ShortSrc, Flushes: flushes,
			CodeBlockCopyLabels: r.CodeBlockCopyLabels, // fork: codeblocks_fork.go
			AttachmentFoldRows:  r.AttachmentFoldRows,  // fork: attachmentfold_fork.go
		}
}
