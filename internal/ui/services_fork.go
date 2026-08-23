package ui

import (
	"context"

	"github.com/gammons/slk/internal/ids"
)

func (m messageAdapter) Preview(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
	if m.fns.Preview == nil {
		return "", "", nil
	}
	return m.fns.Preview(ctx, channelID, ts, threadTS)
}
