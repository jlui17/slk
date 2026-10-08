package slackclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

func (m *mockSlackAPI) GetUserInfoContext(ctx context.Context, user string) (*slack.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.GetUserInfo(user)
}

// The upload stubs only satisfy SlackAPI: upload_fork_test.go exercises
// UploadFileUnshared and ShareFiles against slacktest, through slack-go.
func (m *mockSlackAPI) GetUploadURLExternalContext(ctx context.Context, params slack.GetUploadURLExternalParameters) (*slack.GetUploadURLExternalResponse, error) {
	return &slack.GetUploadURLExternalResponse{}, nil
}

func (m *mockSlackAPI) UploadToURL(ctx context.Context, params slack.UploadToURLParameters) error {
	return nil
}

func (m *mockSlackAPI) CompleteUploadExternalContext(ctx context.Context, params slack.CompleteUploadExternalParameters) (*slack.CompleteUploadExternalResponse, error) {
	return &slack.CompleteUploadExternalResponse{}, nil
}

func TestGetUserProfileContext_HonorsTheContext(t *testing.T) {
	c := &Client{api: &mockSlackAPI{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.GetUserProfileContext(ctx, "U1"); !errors.Is(err, context.Canceled) {
		t.Errorf("GetUserProfileContext on a cancelled ctx: %v, want context.Canceled", err)
	}
}

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

// stars.list puts a starred private channel's or group DM's id under
// "group": it is returned with the channel and DM stars, in order.
func TestGetStarredChannels_ReadsGroupStars(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"items":[
			{"type":"channel","channel":"C1"},
			{"type":"group","group":"G1"},
			{"type":"im","channel":"D1"},
			{"type":"file","file":{"id":"F1"}}
		]}`))
	}))
	defer srv.Close()
	c := &Client{token: "xoxc-test", cookie: "d", apiBaseURL: srv.URL + "/api/"}

	got, err := c.GetStarredChannels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"C1", "G1", "D1"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}
