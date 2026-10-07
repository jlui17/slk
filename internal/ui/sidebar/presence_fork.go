package sidebar

// PresenceOf is the user's live presence ("active" or "away") as the
// websocket last reported it, "" when it never has: Slack reports it for
// the DM peers slk subscribes to.
func (m *Model) PresenceOf(userID string) string { return m.presenceByUser[userID] }
