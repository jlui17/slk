package slackclient

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// slowConvAPI serves users.conversations the way a big or busy
// workspace does: the first request is answered with a 429, then
// the pages arrive one channel each, every page taking delay.
type slowConvAPI struct {
	mockSlackAPI
	retryAfter time.Duration
	pages      int
	delay      time.Duration
	calls      int
}

func (m *slowConvAPI) GetConversationsForUserContext(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	m.calls++
	if m.calls == 1 && m.retryAfter > 0 {
		return nil, "", &slack.RateLimitedError{RetryAfter: m.retryAfter}
	}
	select {
	case <-ctx.Done():
		return nil, "", ctx.Err()
	case <-time.After(m.delay):
	}
	page := len(params.Cursor) + 1 // cursor grows one byte per page
	next := ""
	if page < m.pages {
		next = params.Cursor + "x"
	}
	return []slack.Channel{{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: fmt.Sprintf("C%d", page)}}}}, next, nil
}

// The boot path's users.conversations walk: a 429 sleep and four
// pages that together take longer than the page bound, while no single
// page does. Under one deadline over the whole walk (what connect.go
// did before), this returned context deadline exceeded and boot fell
// back to userBoot's subset of conversations.
func TestGetChannels_PageTimeoutBoundsPagesNotTheWalk(t *testing.T) {
	c := &Client{api: &slowConvAPI{retryAfter: 150 * time.Millisecond, pages: 4, delay: 60 * time.Millisecond}}
	c.SetPageTimeout(100 * time.Millisecond)
	channels, err := c.GetChannels(context.Background())
	if err != nil || len(channels) != 4 {
		t.Fatalf("GetChannels = %d channels, err %v; want all 4 pages", len(channels), err)
	}
}

// A page that never answers still fails at the page bound, so a wedged
// server cannot hang the boot.
func TestGetChannels_HungPageFailsAtPageTimeout(t *testing.T) {
	c := &Client{api: &slowConvAPI{pages: 2, delay: time.Hour}}
	c.SetPageTimeout(50 * time.Millisecond)
	start := time.Now()
	_, err := c.GetChannels(context.Background())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context deadline exceeded", err)
	}
	if took := time.Since(start); took > time.Second {
		t.Errorf("hung page took %s to fail, want about the 50ms bound", took)
	}
}
