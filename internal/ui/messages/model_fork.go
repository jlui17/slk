package messages

// ReplaceMessageContent applies a message_changed event: the text and
// the edited mark as UpdateMessageInPlace sets them, and the blocks and
// attachments the event carries too. Slack delivers a link's unfurl
// this way, after the post, so the text alone would lose the card.
func (m *Model) ReplaceMessageContent(changed MessageItem) bool {
	if !m.UpdateMessageInPlace(changed.TS, changed.Text) {
		return false
	}
	for i := range m.messages {
		if m.messages[i].TS == changed.TS {
			m.messages[i].Blocks = changed.Blocks
			m.messages[i].LegacyAttachments = changed.LegacyAttachments
			m.messages[i].Attachments = changed.Attachments
		}
	}
	return true
}
