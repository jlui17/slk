package threadsview

// MarkByThreadTSRead clears the local Unread flag on the summary matching
// (channelID, threadTS), regardless of whether it is the currently selected
// row. Returns true when a flag was actually flipped, so callers can refresh
// dependent state (sidebar threads-row badge). This is
// presentation-only and does not touch Slack server state.
func (m *Model) MarkByThreadTSRead(channelID, threadTS string) bool {
	if channelID == "" || threadTS == "" {
		return false
	}
	for i := range m.summaries {
		if m.summaries[i].ChannelID == channelID && m.summaries[i].ThreadTS == threadTS {
			if !m.summaries[i].Unread {
				return false
			}
			m.summaries[i].Unread = false
			m.dirty()
			return true
		}
	}
	return false
}
