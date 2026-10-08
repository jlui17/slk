package messages

import "github.com/gammons/slk/internal/ui/messages/blockkit"

// SetHeaderRow sets a row the header draws under the channel name, as
// row(width) exactly width cells wide: an app DM's "Home   Messages"
// tabs. nil removes it.
func (m *Model) SetHeaderRow(row func(width int) string) {
	if row == nil && m.headerRow == nil {
		return
	}
	m.headerRow = row
	m.chromeCacheValid = false
	m.dirty()
}

func (m *Model) headerRowLine(width int) string {
	if m.headerRow == nil {
		return ""
	}
	return "\n" + m.headerRow(width)
}

// HomeBlockKitContext renders an app's Home tab the way this pane
// renders a message's blocks: the same user, channel and emoji names.
// Images draw as their links.
func (m *Model) HomeBlockKitContext() blockkit.Context {
	ctx := m.blockkitContext(MessageItem{}, m.userNames.Current(), m.channelNames)
	ctx.Fetcher = nil
	return ctx
}
