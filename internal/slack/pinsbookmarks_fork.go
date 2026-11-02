package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/slackhttp"
)

// The calls in this file read a conversation's pins (pins.list) and
// bookmarks (bookmarks.list), for the pins and bookmarks dialog, and
// pass on the pin count conversations.history answers with.

// SetPinCountNotify registers fn to hear the pin count of each channel
// whose history GetHistory reads, and when the read began. Slack answers
// conversations.history with pin_count, so the count costs no call of
// its own. fn runs on the fetching goroutine. Set before the client is
// shared across goroutines.
func (c *Client) SetPinCountNotify(fn func(channelID string, count int, started time.Time)) {
	c.pinCountNotify = fn
}

func (c *Client) notifyPinCount(channelID string, count int, started time.Time) {
	if c.pinCountNotify != nil {
		c.pinCountNotify(channelID, count, started)
	}
}

// PinCount reads how many pins channelID has, from a one-message
// conversations.history read (Tier 3, where pins.list is Tier 2): for a
// channel whose history slk does not read when it opens.
// ctx bounds nothing yet: slk's SlackAPI reads history without one, as
// GetHistory does.
func (c *Client) PinCount(_ context.Context, channelID string) (int, error) {
	resp, err := c.api.GetConversationHistory(&slack.GetConversationHistoryParameters{ChannelID: channelID, Limit: 1})
	if err != nil {
		return 0, err
	}
	return resp.PinCount, nil
}

// Pin is one pinned message or file of a conversation.
type Pin struct {
	// Created is when it was pinned, in Unix seconds.
	Created int64
	// TS and ThreadTS are the pinned message's; both are empty for a
	// pinned file.
	TS, ThreadTS string
	// UserID is the message's author; Username is the name a bot posts
	// under, for a message without a user.
	UserID, Username string
	Text             string
	// FileName is the pinned file's name, or the first file's name of a
	// pinned message.
	FileName  string
	Permalink string
}

// Bookmark is one link in a conversation's bookmarks.
type Bookmark struct {
	Title, Link string
	// Folder is the title of the folder the link is in, "" for a link
	// at the top level.
	Folder string
}

// pinsResponse is pins.list's answer.
type pinsResponse struct {
	Items []struct {
		Type    string `json:"type"`
		Created int64  `json:"created"`
		Message *struct {
			TS         string `json:"ts"`
			ThreadTS   string `json:"thread_ts"`
			User       string `json:"user"`
			Username   string `json:"username"`
			BotProfile *struct {
				Name string `json:"name"`
			} `json:"bot_profile"`
			Text      string       `json:"text"`
			Permalink string       `json:"permalink"`
			Files     []pinnedFile `json:"files"`
		} `json:"message"`
		File *pinnedFile `json:"file"`
	} `json:"items"`
}

type pinnedFile struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	Permalink string `json:"permalink"`
}

func (f pinnedFile) name() string {
	if f.Name != "" {
		return f.Name
	}
	return f.Title
}

// ListPins returns channelID's pins, the most recently pinned first.
// Slack limiting requests is a *slack.RateLimitedError.
func (c *Client) ListPins(ctx context.Context, channelID string) ([]Pin, error) {
	var resp pinsResponse
	if err := c.readList(ctx, "pins.list", url.Values{"channel": {channelID}}, &resp); err != nil {
		return nil, err
	}
	pins := make([]Pin, 0, len(resp.Items))
	for _, it := range resp.Items {
		p := Pin{Created: it.Created}
		switch {
		case it.Message != nil:
			m := it.Message
			p.TS, p.ThreadTS, p.UserID, p.Username = m.TS, m.ThreadTS, m.User, m.Username
			if p.Username == "" && m.BotProfile != nil {
				p.Username = m.BotProfile.Name
			}
			p.Text, p.Permalink = m.Text, m.Permalink
			if len(m.Files) > 0 {
				p.FileName = m.Files[0].name()
			}
		case it.File != nil:
			p.FileName, p.Permalink = it.File.name(), it.File.Permalink
		default:
			continue
		}
		pins = append(pins, p)
	}
	sort.SliceStable(pins, func(i, j int) bool { return pins[i].Created > pins[j].Created })
	return pins, nil
}

// bookmarksResponse is bookmarks.list's answer.
type bookmarksResponse struct {
	Bookmarks []struct {
		ID       string `json:"id"`
		Title    string `json:"title"`
		Link     string `json:"link"`
		Type     string `json:"type"`
		Rank     string `json:"rank"`
		ParentID string `json:"parent_id"`
	} `json:"bookmarks"`
}

// bookmarksReason is the _x_reason the web client sends on
// bookmarks.list (testdata/official-request-shape.json in slackhttp).
const bookmarksReason = "bookmarks-store/conditional-fetching"

// ListBookmarks returns channelID's bookmarks in Slack's rank order. A
// folder is not returned itself: its links take its place, in their own
// rank order, with Folder set to its title. An entry without a link is
// left out: Slack lists one of type pinned_message, with no title or
// link, in a channel with pins. Slack limiting requests is a
// *slack.RateLimitedError.
func (c *Client) ListBookmarks(ctx context.Context, channelID string) ([]Bookmark, error) {
	var resp bookmarksResponse
	if err := c.readList(slackhttp.WithReason(ctx, bookmarksReason), "bookmarks.list", url.Values{"channel_id": {channelID}}, &resp); err != nil {
		return nil, err
	}
	all := resp.Bookmarks
	sort.SliceStable(all, func(i, j int) bool { return all[i].Rank < all[j].Rank })
	folders := map[string]string{}
	for _, b := range all {
		if b.Type == "folder" {
			folders[b.ID] = b.Title
		}
	}
	children := map[string][]Bookmark{}
	for _, b := range all {
		if folder, ok := folders[b.ParentID]; ok && b.Link != "" {
			children[b.ParentID] = append(children[b.ParentID], Bookmark{Title: b.Title, Link: b.Link, Folder: folder})
		}
	}
	var out []Bookmark
	for _, b := range all {
		_, inFolder := folders[b.ParentID]
		switch {
		case b.Type == "folder":
			out = append(out, children[b.ID]...)
		case b.Link != "" && !inFolder:
			out = append(out, Bookmark{Title: b.Title, Link: b.Link})
		}
	}
	return out, nil
}

// readList posts one of this file's reads and decodes its answer into
// v: Slack's refusal as refusalOf has it, but a 429 or a "ratelimited"
// refusal as a *slack.RateLimitedError.
func (c *Client) readList(ctx context.Context, method string, form url.Values, v any) error {
	raw, err := c.postForm(ctx, method, form)
	var rl *rateLimitError
	if errors.As(err, &rl) {
		return &slack.RateLimitedError{RetryAfter: rl.retryAfter}
	}
	if err != nil {
		return err
	}
	if err := refusalOf(method, raw); err != nil {
		var refused slack.SlackErrorResponse
		if errors.As(err, &refused) && refused.Err == "ratelimited" {
			return &slack.RateLimitedError{}
		}
		return fmt.Errorf("%s: %w", method, err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("parsing %s: %w", method, err)
	}
	return nil
}

// PinsHandler is an EventHandler that also hears pin_added and
// pin_removed: a pin was added to (added) or removed from channelID.
type PinsHandler interface {
	OnPinsChanged(channelID string, added bool)
}

func dispatchPinEvent(data []byte, handler EventHandler) {
	h, ok := handler.(PinsHandler)
	if !ok {
		return
	}
	var evt struct {
		Type      string `json:"type"`
		ChannelID string `json:"channel_id"`
	}
	if json.Unmarshal(data, &evt) != nil || evt.ChannelID == "" {
		return
	}
	h.OnPinsChanged(evt.ChannelID, evt.Type == "pin_added")
}
