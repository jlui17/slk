package main

import (
	"testing"
	"time"

	"github.com/slack-go/slack"
)

func TestPreviewCache(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newPreviewCache()
	c.now = func() time.Time { return now }

	if _, ok := c.Get("C1", "1.0"); ok {
		t.Fatal("hit on an empty cache")
	}
	c.Put("C1", "1.0", slack.Message{Msg: slack.Msg{User: "U1", Text: "hello"}})
	if m, ok := c.Get("C1", "1.0"); !ok || m.User != "U1" || m.Text != "hello" {
		t.Errorf("Get = (%q, %q, %v), want (U1, hello, true)", m.User, m.Text, ok)
	}

	c.Put("C1", "2.0", slack.Message{})
	if m, ok := c.Get("C1", "2.0"); !ok || m.User != "" || m.Text != "" {
		t.Errorf("not-found entry = (%q, %q, %v), want cached empty hit", m.User, m.Text, ok)
	}

	now = now.Add(previewCacheTTL + time.Second)
	if _, ok := c.Get("C1", "1.0"); ok {
		t.Error("expired entry served")
	}
}

// An entry keeps what a preview is served from and drops the rest of
// the message.
func TestPreviewCache_KeepsOnlyWhatAPreviewNeeds(t *testing.T) {
	c := newPreviewCache()
	c.Put("C1", "1.0", slack.Message{Msg: slack.Msg{
		BotID: "B1", Username: "Robo [nightly export]", Text: "hello", User: "U1",
		Files: []slack.File{{ID: "F1"}}, Attachments: []slack.Attachment{{Text: "card"}},
	}})
	m, _ := c.Get("C1", "1.0")
	if m.User != "U1" || m.BotID != "B1" || m.Username != "Robo [nightly export]" || m.Text != "hello" {
		t.Errorf("entry = %+v, want its user, bot id, username and text", m.Msg)
	}
	if len(m.Files) != 0 || len(m.Attachments) != 0 {
		t.Error("entry kept the message's files or attachments")
	}
}
