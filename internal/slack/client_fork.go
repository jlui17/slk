package slackclient

import (
	"context"
	"fmt"

	"github.com/slack-go/slack"
)

// GetMessageAt fetches the single channel-level message with exactly
// ts, or nil when no such message exists. Thread replies are invisible
// to conversations.history; fetch those via GetReplies with the
// thread's parent ts.
func (c *Client) GetMessageAt(ctx context.Context, channelID, ts string) (*slack.Message, error) {
	resp, err := c.api.GetConversationHistory(&slack.GetConversationHistoryParameters{
		ChannelID: channelID,
		Latest:    ts,
		Inclusive: true,
		Limit:     1,
	})
	if err != nil {
		return nil, fmt.Errorf("getting message at %s: %w", ts, err)
	}
	if len(resp.Messages) == 0 || resp.Messages[0].Timestamp != ts {
		return nil, nil
	}
	return &resp.Messages[0], nil
}

// GetReplyAt fetches the single message with exactly ts from the
// thread rooted at threadTS, or nil when no such message exists. One
// conversations.replies call, no pagination: latest=ts inclusive with
// limit 2, because Slack may return the thread parent alongside the
// requested page; the scan makes the result independent of whether it
// does.
func (c *Client) GetReplyAt(ctx context.Context, channelID, threadTS, ts string) (*slack.Message, error) {
	msgs, _, _, err := c.api.GetConversationReplies(&slack.GetConversationRepliesParameters{
		ChannelID: channelID,
		Timestamp: threadTS,
		Latest:    ts,
		Inclusive: true,
		Limit:     2,
	})
	if err != nil {
		return nil, fmt.Errorf("getting reply at %s: %w", ts, err)
	}
	for i := range msgs {
		if msgs[i].Timestamp == ts {
			return &msgs[i], nil
		}
	}
	return nil, nil
}
