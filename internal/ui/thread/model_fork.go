package thread

import (
	"slices"

	"github.com/gammons/slk/internal/ui/messages"
)

// ReplaceMessageContent applies a message_changed event to the reply or
// the parent it names: the text and the edited mark as
// UpdateMessageInPlace and UpdateParentInPlace set them, and the blocks
// and attachments the event carries too. Slack delivers a link's unfurl
// this way, after the post, so the text alone would lose the card.
func (m *Model) ReplaceMessageContent(changed messages.MessageItem) bool {
	replace := func(msg *messages.MessageItem) {
		msg.Blocks = changed.Blocks
		msg.LegacyAttachments = changed.LegacyAttachments
		msg.Attachments = changed.Attachments
	}
	found := false
	if m.UpdateParentInPlace(changed.TS, changed.Text) {
		replace(&m.parent)
		found = true
	}
	if m.UpdateMessageInPlace(changed.TS, changed.Text) {
		for i := range m.replies {
			if m.replies[i].TS == changed.TS {
				replace(&m.replies[i])
			}
		}
		found = true
	}
	return found
}

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
