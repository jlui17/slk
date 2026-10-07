package messages

import (
	"slices"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

var timestampFormat string

// SetTimestampFormat sets the clock format a linked message's card
// header uses: the config's timestamp_format, as message rows wear it.
func SetTimestampFormat(format string) { timestampFormat = format }

// TimestampFormat is the layout SetTimestampFormat set, the default
// "3:04 PM" until it is called.
func TimestampFormat() string {
	if timestampFormat == "" {
		return "3:04 PM"
	}
	return timestampFormat
}

// CardContext is what a pane hands blockkit for one message's
// attachment cards.
func CardContext(expanded bool, channelNames map[string]string) blockkit.CardContext {
	return blockkit.CardContext{
		Expanded:     expanded,
		ChannelNames: channelNames,
		Now:          nowFunc(),
		TimeFormat:   timestampFormat,
	}
}

// OffsetAttachmentFoldRows moves blockkit's fold rows into the rows of
// the entry whose attachments start `by` rows down.
func OffsetAttachmentFoldRows(rows []int, by int) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r + by
	}
	return out
}

func (m *Model) takeAttachmentFoldRows() []int {
	rows := m.attachmentFoldRows
	m.attachmentFoldRows = nil
	return rows
}

// freshenCache brings the cache up to date the way View does. Events
// drop the cache between draws; a key or a click that lands before the
// next draw must still find the fold rows.
func (m *Model) freshenCache() {
	switch {
	case m.cacheWidth <= 0: // never drawn
	case m.cache == nil || m.cacheMsgLen != len(m.messages):
		m.buildCache(m.cacheWidth)
	case len(m.staleEntries) > 0:
		m.partialRebuild(m.cacheWidth)
	}
}

// ToggleAttachmentFold expands or collapses every attachment card of
// the selected message. It reports false, and does nothing, when the
// message has no card long enough to fold.
func (m *Model) ToggleAttachmentFold() bool {
	m.freshenCache()
	for _, e := range m.cache {
		if e.msgIdx != m.selected || len(e.attachmentFoldRows) == 0 {
			continue
		}
		ts := m.messages[e.msgIdx].TS
		if m.expandedAttachments == nil {
			m.expandedAttachments = map[string]bool{}
		}
		m.expandedAttachments[ts] = !m.expandedAttachments[ts]
		if m.staleEntries == nil {
			m.staleEntries = make(map[string]struct{})
		}
		m.staleEntries[ts] = struct{}{}
		// The message changes height: the next View brings it back into view.
		m.hasSnapped = false
		m.dirty()
		return true
	}
	return false
}

// ToggleAttachmentFoldAt handles a click at pane-local row y, the frame
// ClickAt takes: on a card's fold row it selects that message and
// toggles its cards.
func (m *Model) ToggleAttachmentFoldAt(y int) bool {
	if y < m.chromeHeight {
		return false
	}
	m.freshenCache()
	line := y - m.chromeHeight + m.yOffset
	for i, e := range m.cache {
		if slices.Contains(e.attachmentFoldRows, line-m.entryOffsets[i]) {
			m.ClickAt(y)
			return m.ToggleAttachmentFold()
		}
	}
	return false
}
