package ui

import tea "charm.land/bubbletea/v2"

// openConversationCmd opens the DM or group DM with userIDs, as the
// new-message picker's submit does: reduceNewMessagePicker switches to
// it once Slack has opened it. It bumps the in-flight ID and clears
// cancellation before dispatch, so a fresh result is honored.
func (a *App) openConversationCmd(userIDs []string) tea.Cmd {
	a.newMessageInFlightID++
	a.newMessageCancelled = false
	return teaCmd(a.channels.OpenConversation(userIDs, a.newMessageInFlightID))
}
