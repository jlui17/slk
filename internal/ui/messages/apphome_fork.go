package messages

import (
	"strings"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// HeaderSlot is a row the header draws under the channel name. The
// rows draw in slot order.
type HeaderSlot int

const (
	// HeaderTabs is an app DM's "Home   Messages" tabs.
	HeaderTabs HeaderSlot = iota
	// HeaderCounts is the count of a channel's pins and bookmarks.
	HeaderCounts
	headerSlots
)

// SetHeaderRow sets the row of slot, as row(width) exactly width cells
// wide. nil removes it.
func (m *Model) SetHeaderRow(slot HeaderSlot, row func(width int) string) {
	if row == nil && m.headerRows[slot] == nil {
		return
	}
	m.headerRows[slot] = row
	m.chromeCacheValid = false
	m.dirty()
}

func (m *Model) headerRowLine(width int) string {
	var b strings.Builder
	for _, row := range m.headerRows {
		if row != nil {
			b.WriteString("\n" + row(width))
		}
	}
	return b.String()
}

// HomeBlockKitContext renders an app's Home tab the way this pane
// renders a message's blocks: the same user, channel and emoji names.
// Images draw as their links.
func (m *Model) HomeBlockKitContext() blockkit.Context {
	ctx := BlockkitContextFork(MessageItem{}, m.renderContext(), nil)
	ctx.Fetcher = nil
	return ctx
}
