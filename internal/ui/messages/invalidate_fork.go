package messages

import (
	"reflect"
	"slices"
	"strings"
)

// The channel pane's targeted invalidations: each event re-renders only
// the entries whose rows it changes, through markEntryStale (partial
// rebuild) or markRelayout (the entry list rebuilt around kept
// entries). invalidate_oracle_fork_test.go holds every one of them to a
// full rebuild, byte for byte.

// markUserStale marks the entries a change of userID's name changes:
// the messages they wrote, and every message whose text, blocks or
// attachments carry their ID (a mention).
func (m *Model) markUserStale(userID string) {
	for i := range m.messages {
		if msg := &m.messages[i]; msg.UserID == userID || RefersTo(msg, userID) {
			m.markMessageStale(msg.TS)
		}
	}
}

// markAuthorStale marks the entries of the messages userID wrote, whose
// headers carry their status.
func (m *Model) markAuthorStale(userID string) {
	for i := range m.messages {
		if m.messages[i].UserID == userID {
			m.markMessageStale(m.messages[i].TS)
		}
	}
}

// markMessageStale is markEntryStale for a message that may have no ts,
// whose entry cannot be found: the cache is dropped.
func (m *Model) markMessageStale(ts string) {
	if ts == "" {
		m.dropCache()
		return
	}
	m.markEntryStale(ts)
}

// RefersTo reports whether userID appears in msg's text, blocks, legacy
// attachments or files: every place a mention the renderer resolves to a
// name can come from. Reactions are left out; a pill shows no names. It
// may report a message that only contains the ID inside a longer one,
// which costs a render, never a stale row.
func RefersTo(msg *MessageItem, userID string) bool {
	return strings.Contains(msg.Text, userID) ||
		stringsContain(reflect.ValueOf(msg.Blocks), userID) ||
		stringsContain(reflect.ValueOf(msg.LegacyAttachments), userID) ||
		stringsContain(reflect.ValueOf(msg.Attachments), userID)
}

// stringsContain reports whether any string reachable from v contains
// s. A walk by reflection, so a block type added later is covered
// without a change here.
func stringsContain(v reflect.Value, s string) bool {
	switch v.Kind() {
	case reflect.String:
		return strings.Contains(v.String(), s)
	case reflect.Struct:
		for i := range v.NumField() {
			if stringsContain(v.Field(i), s) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := range v.Len() {
			if stringsContain(v.Index(i), s) {
				return true
			}
		}
	case reflect.Pointer, reflect.Interface:
		return !v.IsNil() && stringsContain(v.Elem(), s)
	case reflect.Map:
		for it := v.MapRange(); it.Next(); {
			if stringsContain(it.Key(), s) || stringsContain(it.Value(), s) {
				return true
			}
		}
	}
	return false
}

// markNavStale marks the entries a reaction-nav change redraws: the
// selected one, and every entry drawn with the nav highlight since the
// last change (noteNavDrawn), which can differ when the cursor moved
// under an active nav.
func (m *Model) markNavStale() {
	for _, ts := range m.navDrawn {
		m.markMessageStale(ts)
	}
	m.navDrawn = m.navDrawn[:0]
	if m.selected >= 0 && m.selected < len(m.messages) {
		m.markMessageStale(m.messages[m.selected].TS)
	}
}

// noteNavDrawn records that the entry of ts was rendered as the cursor's
// while reaction nav was active: it carries the highlight until it is
// rendered again.
func (m *Model) noteNavDrawn(ts string, isSelected bool) {
	if isSelected && m.nav.Active {
		m.navDrawn = append(m.navDrawn, ts)
	}
}

// markChangedStale prepares SetMessages(msgs) to keep the entry of every
// message msgs carries unchanged: same ts, same content. The others are
// rendered (a new ts has no entry to keep; a changed one is marked
// stale) and the entry list is rebuilt around them. A ts names one
// message, so an entry is never kept across channels. Messages without a
// ts cannot be told apart: the cache is dropped.
func (m *Model) markChangedStale(msgs []MessageItem) {
	if m.cache == nil {
		return
	}
	old := make(map[string]*MessageItem, len(m.messages))
	for i := range m.messages {
		old[m.messages[i].TS] = &m.messages[i]
	}
	if _, ok := old[""]; ok || slices.ContainsFunc(msgs, func(msg MessageItem) bool { return msg.TS == "" }) {
		m.dropCache()
		return
	}
	for i := range msgs {
		if prev, ok := old[msgs[i].TS]; ok && !reflect.DeepEqual(*prev, msgs[i]) {
			m.markEntryStale(msgs[i].TS)
		}
	}
	m.markRelayout()
}
