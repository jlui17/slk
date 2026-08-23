package slackclient

import (
	"context"
	"testing"

	"github.com/slack-go/slack"
)

func TestGetReplyAt(t *testing.T) {
	mock := &mockSlackAPI{
		getConversationRepliesFn: func(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			if params.Timestamp != "1700000001.000000" || params.Latest != "1700000002.000000" ||
				!params.Inclusive || params.Limit != 2 || params.Cursor != "" {
				t.Errorf("params = %+v, want single targeted page", params)
			}
			return []slack.Message{
				{Msg: slack.Msg{Timestamp: "1700000001.000000", Text: "parent msg", User: "U1"}},
				{Msg: slack.Msg{Timestamp: "1700000002.000000", Text: "reply 1", User: "U2"}},
			}, true, "cursor_more", nil
		},
	}
	client := &Client{api: mock}

	m, err := client.GetReplyAt(context.Background(), "C123", "1700000001.000000", "1700000002.000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m == nil || m.Text != "reply 1" {
		t.Fatalf("m = %+v, want reply 1 (no pagination despite hasMore)", m)
	}
}

// A latest= page can return the nearest-older reply when ts itself
// doesn't exist; GetReplyAt must report nil, not that neighbor.
func TestGetReplyAt_MissingTS(t *testing.T) {
	mock := &mockSlackAPI{
		getConversationRepliesFn: func(params *slack.GetConversationRepliesParameters) ([]slack.Message, bool, string, error) {
			return []slack.Message{
				{Msg: slack.Msg{Timestamp: "1700000001.000000", Text: "parent msg", User: "U1"}},
			}, false, "", nil
		},
	}
	client := &Client{api: mock}

	m, err := client.GetReplyAt(context.Background(), "C123", "1700000001.000000", "1700000009.000000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Fatalf("m = %+v, want nil for a ts not in the thread", m)
	}
}
