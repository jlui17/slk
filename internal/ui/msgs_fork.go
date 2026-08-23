package ui

// threadMarkDebounceMsg is delivered after live replies into the open
// thread panel stop arriving for the mark-debounce interval. Carries the
// panel identity at scheduling time plus a generation; if the App's
// pendingThreadMarkGen has advanced past `gen`, the message is dropped —
// a later reply in the burst has scheduled a fresh mark. The TS to mark
// with is resolved at fire time from the panel's newest reply, so one
// surviving tick marks the whole burst read.
type threadMarkDebounceMsg struct {
	channelID string
	threadTS  string
	gen       uint64
}

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
