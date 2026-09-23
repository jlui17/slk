package ui

// LinkPreviewMsg delivers one link-picker row's fetched message
// preview (the slk permalink rows of an `o`/`O` picker). Index is the
// picker row; Gen echoes App.linkPreviewGen at dispatch time (stamped
// UI-side in fetchLinkPreview) and the reducer drops stale
// generations. ChannelID is the permalink's channel (for the row's
// channel prefix); UserID and Text are the target message's sender
// and raw mrkdwn.
type LinkPreviewMsg struct {
	Index     int
	Gen       uint64
	ChannelID string
	UserID    string
	Text      string
}
