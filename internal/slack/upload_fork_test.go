package slackclient

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/slacktest"
	"github.com/slack-go/slack"
)

// uploadTwoAndShare uploads two files with UploadFileUnshared and
// shares them with ShareFiles against the fake backend, the way the
// composer's uploader does, and returns the server for inspection.
func uploadTwoAndShare(t *testing.T, threadTS, caption string) *slacktest.Server {
	t.Helper()
	s := slacktest.New(t)
	s.Handle("/api/files.getUploadURLExternal", `{"ok":true,"upload_url":"https://files.slack.com/upload/v1/abc","file_id":"F0001"}`)
	// Real Slack answers the upload URL with plain text; slack-go never
	// reads this body.
	s.Handle("/upload/v1/abc", "OK - 3")
	s.Handle("/api/files.completeUploadExternal", `{"ok":true,"files":[{"id":"F0001"},{"id":"F0001"}]}`)
	c := NewTestClient("xoxc-test", "d-test", WithInnerTransport(s.Transport()))
	ctx := context.Background()

	var files []slack.FileSummary
	for _, name := range []string{"a.png", "b.png"} {
		f, err := c.UploadFileUnshared(ctx, name, strings.NewReader("png"), 3)
		if err != nil {
			t.Fatalf("UploadFileUnshared(%s): %v", name, err)
		}
		if f.ID != "F0001" {
			t.Fatalf("UploadFileUnshared(%s) id = %q; want F0001", name, f.ID)
		}
		files = append(files, f)
	}
	if err := c.ShareFiles(ctx, slacktest.ChannelID, threadTS, files, caption); err != nil {
		t.Fatalf("ShareFiles: %v", err)
	}
	return s
}

func TestShareFiles_TwoFilesAndCaptionGoOutAsOneMessage(t *testing.T) {
	s := uploadTwoAndShare(t, "", "look at these")

	if got := len(s.RequestsTo("/api/files.getUploadURLExternal")); got != 2 {
		t.Errorf("getUploadURLExternal requests = %d; want 2", got)
	}
	if got := len(s.RequestsTo("/upload/v1/abc")); got != 2 {
		t.Errorf("upload URL requests = %d; want 2", got)
	}
	completes := s.RequestsTo("/api/files.completeUploadExternal")
	if len(completes) != 1 {
		t.Fatalf("completeUploadExternal requests = %d; want 1", len(completes))
	}
	form := completes[0].Form
	var shared []slack.FileSummary
	if err := json.Unmarshal([]byte(form.Get("files")), &shared); err != nil {
		t.Fatalf("files = %q: %v", form.Get("files"), err)
	}
	if len(shared) != 2 {
		t.Errorf("files = %q; want 2 entries", form.Get("files"))
	}
	if got := form.Get("initial_comment"); got != "look at these" {
		t.Errorf("initial_comment = %q; want %q", got, "look at these")
	}
	if got := form.Get("channel_id"); got != slacktest.ChannelID {
		t.Errorf("channel_id = %q; want %s", got, slacktest.ChannelID)
	}
	if form.Has("thread_ts") {
		t.Errorf("thread_ts = %q; want absent outside a thread", form.Get("thread_ts"))
	}
}

func TestShareFiles_InThreadSendsThreadTS(t *testing.T) {
	s := uploadTwoAndShare(t, "1700000000.000100", "in the thread")

	completes := s.RequestsTo("/api/files.completeUploadExternal")
	if len(completes) != 1 {
		t.Fatalf("completeUploadExternal requests = %d; want 1", len(completes))
	}
	if got := completes[0].Form.Get("thread_ts"); got != "1700000000.000100" {
		t.Errorf("thread_ts = %q; want 1700000000.000100", got)
	}
}

func TestShareFiles_FewerSharedThanSentIsAnError(t *testing.T) {
	s := slacktest.New(t)
	s.Handle("/api/files.completeUploadExternal", `{"ok":true,"files":[{"id":"F0001"}]}`)
	c := NewTestClient("xoxc-test", "d-test", WithInnerTransport(s.Transport()))

	err := c.ShareFiles(context.Background(), slacktest.ChannelID, "", []slack.FileSummary{{ID: "F0001"}, {ID: "F0002"}}, "")
	if err == nil || !strings.Contains(err.Error(), "shared 1 of 2") {
		t.Fatalf("ShareFiles err = %v; want one naming 1 of 2 shared", err)
	}
}
