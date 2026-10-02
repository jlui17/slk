package main

import (
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/usernames"
)

// A deleted message's tombstone row still carries its text; the
// preview cache tier must report it as found-but-empty, never serve
// the text or fall through to a doomed network fetch.
func TestCachedMessagePreview(t *testing.T) {
	db := newCacheForTest(t)
	if err := db.UpsertMessage(cache.Message{
		ChannelID: "C1", TS: "1700000000.000100", UserID: "U1", Text: "hello",
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	names := usernames.FromMap(map[string]string{"U1": "dana"})
	sender, text, found := cachedMessagePreview(db, names, nil, "C1", "1700000000.000100")
	if !found || sender != "dana" || text != "hello" {
		t.Errorf("live message = (%q, %q, %v), want (dana, hello, true)", sender, text, found)
	}

	if _, _, found := cachedMessagePreview(db, names, nil, "C1", "1700000000.999999"); found {
		t.Error("missing message reported as found")
	}

	if err := db.DeleteMessage("C1", "1700000000.000100"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	sender, text, found = cachedMessagePreview(db, names, nil, "C1", "1700000000.000100")
	if !found || sender != "" || text != "" {
		t.Errorf("tombstone = (%q, %q, %v), want found with no sender/text", sender, text, found)
	}
}

// The sender of a cached bot post is the name the message pane shows
// for it: the username the post itself carries, which one bot can change
// from post to post, before the one name cached for the bot's id. The
// cache holds two shapes of a bot post: from a history fetch (bot_id and
// username, no user) and from a live event (the bot id as user, nothing
// else).
func TestCachedMessagePreview_BotSender(t *testing.T) {
	db := newCacheForTest(t)
	for ts, raw := range map[string]string{
		"1.000001": `{"type":"message","subtype":"bot_message","bot_id":"B1","username":"Robo [nightly export]","text":"one","ts":"1.000001"}`,
		"1.000002": `{"type":"message","subtype":"bot_message","bot_id":"B1","username":"Robo [cache rename]","text":"two","ts":"1.000002"}`,
		"1.000003": `{"type":"message","user":"B1","text":"three","ts":"1.000003"}`,
	} {
		if err := db.UpsertMessage(cache.Message{ChannelID: "C1", TS: ts, UserID: "B1", Text: "x", RawJSON: raw}); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	names := usernames.FromMap(map[string]string{"B1": "Robo"})
	for ts, want := range map[string]string{
		"1.000001": "Robo [nightly export]",
		"1.000002": "Robo [cache rename]",
		"1.000003": "Robo",
	} {
		if sender, _, _ := cachedMessagePreview(db, names, nil, "C1", ts); sender != want {
			t.Errorf("sender of %s = %q, want %q", ts, sender, want)
		}
	}
}

// A bot post fetched from Slack has no user: its sender is its username,
// where the preview took the empty user and the row showed no sender.
func TestPreviewSender_FetchedBotPost(t *testing.T) {
	names := usernames.FromMap(nil)
	post := slack.Message{Msg: slack.Msg{SubType: "bot_message", BotID: "B1", Username: "Robo [nightly export]", Text: "one"}}
	if got := previewSender(post, names, nil, nil); got != "Robo [nightly export]" {
		t.Errorf("sender = %q, want the post's username", got)
	}
	if got := previewSender(slack.Message{}, names, nil, nil); got != "" {
		t.Errorf("sender of a message that names no one = %q, want none", got)
	}
}

// A fetched preview is cached, its sender's name is not: a sender who
// was only an id when the preview was fetched shows by name once the
// name is known, without another fetch.
func TestFetchedMessagePreview_SenderResolvedWhenServed(t *testing.T) {
	names := usernames.FromMap(nil)
	fetches := 0
	fetch := func() (*slack.Message, error) {
		fetches++
		return &slack.Message{Msg: slack.Msg{User: "U9", Text: "hello"}}, nil
	}
	c := newPreviewCache()
	if sender, text, _ := fetchedMessagePreview(c, fetch, names, nil, nil, "C1", "1.0"); sender != "U9" || text != "hello" {
		t.Fatalf("first preview = (%q, %q), want the id until the name is known", sender, text)
	}
	names.Set("U9", "dana")
	if sender, _, _ := fetchedMessagePreview(c, fetch, names, nil, nil, "C1", "1.0"); sender != "dana" || fetches != 1 {
		t.Errorf("second preview: sender %q after %d fetches, want dana after 1", sender, fetches)
	}
}
