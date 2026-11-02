package main

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/core"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/pinsbookmarks"
)

// pinsBookmarksService is the pins and bookmarks dialog's
// pinsbookmarks.Service, routed to the workspace the channel is in.
type pinsBookmarksService struct{ router *workspaceRouter }

func (s pinsBookmarksService) Pins(ctx context.Context, teamID, channelID string) ([]pinsbookmarks.Pin, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return nil, err
	}
	pins, err := c.ListPins(ctx, channelID)
	if err != nil {
		return nil, coreRateLimit(err)
	}
	out := make([]pinsbookmarks.Pin, len(pins))
	for i, p := range pins {
		out[i] = pinsbookmarks.Pin{
			TS:        p.TS,
			ThreadTS:  p.ThreadTS,
			UserID:    p.UserID,
			Username:  p.Username,
			Text:      p.Text,
			FileName:  p.FileName,
			Permalink: p.Permalink,
		}
	}
	return out, nil
}

func (s pinsBookmarksService) Bookmarks(ctx context.Context, teamID, channelID string) ([]pinsbookmarks.Bookmark, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return nil, err
	}
	bookmarks, err := c.ListBookmarks(ctx, channelID)
	if err != nil {
		return nil, coreRateLimit(err)
	}
	out := make([]pinsbookmarks.Bookmark, len(bookmarks))
	for i, b := range bookmarks {
		out[i] = pinsbookmarks.Bookmark{Title: b.Title, Link: b.Link, Folder: b.Folder}
	}
	return out, nil
}

// coreRateLimit is err as the UI takes it: Slack limiting requests as a
// *core.RateLimitedError, anything else unchanged.
func coreRateLimit(err error) error {
	var rl *slack.RateLimitedError
	if errors.As(err, &rl) {
		return &core.RateLimitedError{RetryAfter: rl.RetryAfter}
	}
	return err
}

func (s pinsBookmarksService) PinCount(ctx context.Context, teamID, channelID string) (int, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return 0, err
	}
	n, err := c.PinCount(ctx, channelID)
	return n, coreRateLimit(err)
}

// notifyPinCounts tells the UI, through send, the pin count of each
// channel whose history client reads, for the count line under the
// channel's name. The workspace is the team auth.test named, read when
// the count comes: the key the rtm handler and the router use.
func notifyPinCounts(client *slackclient.Client, send func(tea.Msg)) {
	client.SetPinCountNotify(func(channelID string, count int, started time.Time) {
		send(ui.PinCountMsg{TeamID: client.TeamID(), ChannelID: channelID, Count: count, Started: started})
	})
}

var _ slackclient.PinsHandler = (*rtmEventHandler)(nil)

// OnPinsChanged tells the UI a pin was added to or removed from
// channelID.
func (h *rtmEventHandler) OnPinsChanged(channelID string, added bool) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.PinsChangedMsg{TeamID: h.workspaceID, ChannelID: channelID, Added: added})
}
