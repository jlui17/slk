package core

import (
	"context"

	"github.com/gammons/slk/internal/ids"
)

// MessagePreviewFetchFunc resolves the sender ID and raw mrkdwn text
// of the message a Slack permalink points at, for the link picker's
// preview rows. Cache-first: the local message cache, then the API on
// a miss. threadTS is the permalink's thread_ts ("" for channel-level
// links); the network path needs it because conversations.history
// never returns thread replies. ("", "", nil) means the message
// couldn't be resolved; callers keep their fallback row.
type MessagePreviewFetchFunc func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (userID, text string, err error)

func (m messageAdapter) Preview(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
	if m.fns.Preview == nil {
		return "", "", nil
	}
	return m.fns.Preview(ctx, channelID, ts, threadTS)
}
