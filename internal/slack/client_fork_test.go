package slackclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

// blockingConvAPI never answers until the caller's ctx dies, standing
// in for a wedged users.conversations request.
type blockingConvAPI struct{ mockSlackAPI }

func (m *blockingConvAPI) GetConversationsForUserContext(ctx context.Context, params *slack.GetConversationsForUserParameters) ([]slack.Channel, string, error) {
	<-ctx.Done()
	return nil, "", ctx.Err()
}

func TestGetChannels_ForwardsContextIntoTheRequest(t *testing.T) {
	c := &Client{api: &blockingConvAPI{}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := c.GetChannels(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetChannels err = %v, want the caller's deadline to reach the request", err)
	}
}

// authCtxAPI records the ctx auth.test was called with.
type authCtxAPI struct {
	mockSlackAPI
	got context.Context
}

func (m *authCtxAPI) AuthTestContext(ctx context.Context) (*slack.AuthTestResponse, error) {
	m.got = ctx
	return &slack.AuthTestResponse{TeamID: "T1", UserID: "U1", URL: "https://x.slack.com/"}, nil
}

func TestConnect_ForwardsContextToAuthTest(t *testing.T) {
	api := &authCtxAPI{}
	c := &Client{api: api}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if api.got == nil {
		t.Fatal("auth.test never saw a ctx")
	}
	if _, ok := api.got.Deadline(); !ok {
		t.Error("Connect dropped the caller's deadline on the way to auth.test")
	}
}

func TestGetUnreadCounts_HonorsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	t.Cleanup(func() {
		close(release)
		srv.Close()
	})

	c := &Client{
		token:      "xoxc-test",
		apiBaseURL: srv.URL + "/api/",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, err := c.GetUnreadCounts(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("GetUnreadCounts against a hung server: err = %v, want ctx deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("returned after %s; the ctx cap did not take effect", elapsed)
	}
}
