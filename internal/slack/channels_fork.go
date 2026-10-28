package slackclient

import (
	"context"
	"time"

	"github.com/slack-go/slack"
)

// SetPageTimeout bounds each users.conversations page GetChannels
// fetches. The bound is per page, not per walk: a big account pages
// 200 conversations at a time and sleeps in place on every 429, so one
// deadline over the whole walk cuts off a healthy fetch, while a page
// that hangs still fails within d. Zero (the default) leaves pages
// bounded only by the caller's ctx. Set before the client is shared
// across goroutines.
func (c *Client) SetPageTimeout(d time.Duration) {
	c.pageTimeout = d
}

func (c *Client) conversationsPage(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	if c.pageTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.pageTimeout)
		defer cancel()
	}
	return c.api.GetConversationsForUserContext(ctx, params)
}
