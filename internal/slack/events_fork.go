package slackclient

import "encoding/json"

// dispatchChangedEphemeral hands an app's replacement of its ephemeral
// (a message_changed carrying is_ephemeral at either level) to
// OnEphemeralMessage with the ts it replaces, as dispatchEphemeral does
// for a new one; the UI shows it in place of the one it replaces.
// Upstream drops these.
func dispatchChangedEphemeral(data []byte, msg wsMessageEvent, handler EventHandler) {
	inner := msg.Message
	var raw struct {
		Message json.RawMessage `json:"message"`
	}
	var actionIDs [][]string
	if err := json.Unmarshal(data, &raw); err == nil && len(raw.Message) > 0 {
		actionIDs = legacyActionIDs(raw.Message)
	}
	handler.OnEphemeralMessage(EphemeralMessage{
		ChannelID:   msg.Channel,
		UserID:      inner.User,
		BotID:       inner.BotID,
		Username:    inner.Username,
		TS:          inner.TS,
		ThreadTS:    inner.ThreadTS,
		Text:        inner.Text,
		Blocks:      inner.Blocks,
		Attachments: inner.Attachments,
		ActionIDs:   actionIDs,
	})
}
