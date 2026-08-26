package slackclient

import (
	"context"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

func TestRateLimitWait_DefaultsWhenRetryAfterMissing(t *testing.T) {
	c := &Client{}
	if got := c.rateLimitWait(&slack.RateLimitedError{}); got != 5*time.Second {
		t.Errorf("wait with no Retry-After = %s, want 5s", got)
	}
	if got := c.rateLimitWait(&slack.RateLimitedError{RetryAfter: 7 * time.Second}); got != 7*time.Second {
		t.Errorf("wait with Retry-After 7s = %s, want the header honored as-is", got)
	}
}

func TestRateLimitWait_NotifiesBeforeSleeping(t *testing.T) {
	var got []time.Duration
	c := &Client{}
	c.SetRateLimitNotify(func(wait time.Duration) { got = append(got, wait) })
	c.rateLimitWait(&slack.RateLimitedError{RetryAfter: 2 * time.Second})
	if len(got) != 1 || got[0] != 2*time.Second {
		t.Errorf("notify observed %v, want [2s]", got)
	}
}

// rateLimitedConvAPI answers the first users.conversations page with a
// 429 and the second with one channel, so a test can watch GetChannels
// retry the same page through the rateLimitWait helper.
type rateLimitedConvAPI struct {
	mockSlackAPI
	calls int
}

func (m *rateLimitedConvAPI) GetConversationsForUserContext(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	m.calls++
	if m.calls == 1 {
		return nil, "", &slack.RateLimitedError{RetryAfter: time.Millisecond}
	}
	return []slack.Channel{{}}, "", nil
}

func TestGetChannels_RateLimitSleepIsObservable(t *testing.T) {
	api := &rateLimitedConvAPI{}
	c := &Client{api: api}
	var notified []time.Duration
	c.SetRateLimitNotify(func(wait time.Duration) { notified = append(notified, wait) })

	channels, err := c.GetChannels(context.Background())
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}
	if len(channels) != 1 || api.calls != 2 {
		t.Errorf("channels=%d calls=%d, want the rate-limited page retried once", len(channels), api.calls)
	}
	if len(notified) != 1 || notified[0] != time.Millisecond {
		t.Errorf("notify observed %v, want [1ms] — the sleep the retry took", notified)
	}
}

