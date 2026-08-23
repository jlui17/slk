package ui

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

// ReloadFunc forces every workspace's websocket to reconnect now (the
// manual reload, slk's cmd+r analog): pending backoff waits are
// skipped and the reconnect catch-up dedupe gates are reset so the
// catch-up pass runs even right after a natural reconnect.
type ReloadFunc func()
