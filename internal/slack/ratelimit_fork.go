package slackclient

import (
	"time"

	"github.com/slack-go/slack"
)

// rateLimitRetryDefault is the sleep before retrying a 429 that came
// without a Retry-After duration. Slack normally sends the header, so
// this covers only the malformed case; the old 30s default read as a
// silent freeze on the boot path.
const rateLimitRetryDefault = 5 * time.Second

// SetRateLimitNotify registers fn to observe every 429 retry sleep
// this client performs (the paginated calls that retry in place). fn
// runs on the fetching goroutine, before the sleep starts, so it must
// be safe to call concurrently. Set before the client is shared
// across goroutines.
func (c *Client) SetRateLimitNotify(fn func(wait time.Duration)) {
	c.rateLimitNotify = fn
}

func (c *Client) rateLimitWait(err *slack.RateLimitedError) time.Duration {
	wait := err.RetryAfter
	if wait <= 0 {
		wait = rateLimitRetryDefault
	}
	if c.rateLimitNotify != nil {
		c.rateLimitNotify(wait)
	}
	return wait
}
