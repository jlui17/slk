package thread

import (
	"slices"

	"github.com/gammons/slk/internal/ui/messages"
)

// ReplaceEphemeral puts msg, an ephemeral reply its app replaced, in
// place of the reply with its ts, whole, as the next reload lays it in:
// Slack marks no ephemeral edited. It reports whether the reply was
// there.
func (m *Model) ReplaceEphemeral(msg messages.MessageItem) bool {
	i := slices.IndexFunc(m.replies, func(old messages.MessageItem) bool { return old.TS == msg.TS })
	if i < 0 {
		return false
	}
	m.replies[i] = msg
	m.InvalidateCache()
	return true
}
