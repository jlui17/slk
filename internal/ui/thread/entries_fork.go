package thread

import (
	"github.com/gammons/slk/internal/bubbles/entrylist"

	"github.com/gammons/slk/internal/ui/messages"
)

// forkEntryAt returns the list entry drawn at content line `line`
// (yOffset already added), the line it starts on, and its payload: the
// parent's (always list entry 0) or its reply's. ok is false on a
// decoration row (a rule, a day divider, the landmark) or past the end.
func (m *Model) forkEntryAt(line int) (e entrylist.Entry, start int, payload *messages.EntryPayload, ok bool) {
	offsets := m.list.Offsets()
	for i, e := range m.list.Entries() {
		if line < offsets[i] || line >= offsets[i]+len(e.Lines) {
			continue
		}
		switch {
		case i == 0:
			return e, offsets[i], &m.parentEntry, true
		case e.Ref >= 0 && e.Ref < len(m.cache):
			return e, offsets[i], &m.cache[e.Ref], true
		}
		return e, offsets[i], nil, false
	}
	return entrylist.Entry{}, 0, nil, false
}
