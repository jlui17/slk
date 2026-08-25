package slackclient

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
	"github.com/gammons/slk/internal/slacktest"
	"github.com/slack-go/slack"
)

// harnessHandler is mockEventHandler with a channel-observed OnMessage,
// so the test can wait for the WS read goroutine without a data race.
type harnessHandler struct {
	mockEventHandler
	texts chan string
}

func (h *harnessHandler) OnMessage(channelID, userID, ts, text, threadTS, subtype string, edited bool, files []slack.File, blocks slack.Blocks, attachments []slack.Attachment, botID, username string) {
	h.texts <- text
}

// TestSlacktestHarness_BootAndEventsWithNoCredentials is the
// internal/slacktest harness's proof and usage documentation: a real
// *Client, wired through the fork seams, runs Connect, the boot-path
// fetches, and the event WebSocket against the fake backend with no
// real credentials and no network.
func TestSlacktestHarness_BootAndEventsWithNoCredentials(t *testing.T) {
	s := slacktest.New(t)
	c := NewTestClient("xoxc-test", "d-test",
		WithInnerTransport(s.Transport()),
		WithWSDialer(s.WSDialer()),
	)
	ctx := context.Background()

	// (a) Connect: auth.test round-trips through the fake.
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.TeamID() != slacktest.TeamID || c.UserID() != slacktest.SelfID {
		t.Fatalf("Connect: teamID=%q userID=%q; want %s / %s",
			c.TeamID(), c.UserID(), slacktest.TeamID, slacktest.SelfID)
	}
	// The production URL shape survived: Connect derived a *.slack.com
	// API base from the corpus; only the dial was redirected.
	if got, want := c.apiBaseURL, "https://slacktest.slack.com/api/"; got != want {
		t.Errorf("apiBaseURL after Connect = %q; want %q", got, want)
	}

	// (b) Boot fetches parse the canned corpus.
	ub, err := boot.UserBoot(ctx, c.PostForm)
	if err != nil {
		t.Fatalf("UserBoot: %v", err)
	}
	if len(ub.Channels) != 1 || ub.Channels[0].ID != slacktest.ChannelID {
		t.Errorf("UserBoot channels = %+v; want one with ID %s", ub.Channels, slacktest.ChannelID)
	}
	if len(ub.IMs) != 1 || ub.IMs[0].UserID != slacktest.PeerID {
		t.Errorf("UserBoot ims = %+v; want one with user %s", ub.IMs, slacktest.PeerID)
	}
	if ub.Self.ID != slacktest.SelfID || ub.Team.ID != slacktest.TeamID {
		t.Errorf("UserBoot self=%q team=%q; want %s / %s",
			ub.Self.ID, ub.Team.ID, slacktest.SelfID, slacktest.TeamID)
	}

	view, err := boot.ConversationsView(ctx, c.PostForm, slacktest.ChannelID)
	if err != nil {
		t.Fatalf("ConversationsView: %v", err)
	}
	if view.Channel.ID != slacktest.ChannelID || len(view.History.Messages) != 1 {
		t.Errorf("ConversationsView channel=%q messages=%d; want %s / 1",
			view.Channel.ID, len(view.History.Messages), slacktest.ChannelID)
	}

	// A per-test Handle override beats the default corpus.
	s.Handle("/api/client.counts", `{"ok":true,"channels":[{"id":"`+slacktest.ChannelID+`","has_unreads":true,"mention_count":7,"last_read":"1700000001.000100"}],"threads":{"has_unreads":true,"unread_count":3,"mention_count":1}}`)
	unreads, threads, err := c.GetUnreadCounts()
	if err != nil {
		t.Fatalf("GetUnreadCounts: %v", err)
	}
	if len(unreads) != 1 || unreads[0].Count != 7 {
		t.Errorf("unreads = %+v; want one with Count 7 from the override", unreads)
	}
	if !threads.HasUnreads || threads.UnreadCount != 3 {
		t.Errorf("threads = %+v; want HasUnreads with UnreadCount 3", threads)
	}

	chans, err := c.GetChannels(ctx)
	if err != nil {
		t.Fatalf("GetChannels: %v", err)
	}
	if len(chans) != 2 {
		t.Errorf("GetChannels returned %d conversations; want 2", len(chans))
	}

	ec := edge.New("xoxc-test", slacktest.TeamID, c.HTTPClient())
	info, err := ec.ChannelsInfo(ctx, slacktest.TeamID, map[string]int64{slacktest.ChannelID: 0})
	if err != nil {
		t.Fatalf("edge ChannelsInfo: %v", err)
	}
	if len(info.Channels) != 1 || len(info.MemberChannels) != 1 || info.MemberChannels[0] != slacktest.ChannelID {
		t.Errorf("edge ChannelsInfo = %+v; want %s changed and a member", info, slacktest.ChannelID)
	}
	users, err := ec.UsersInfo(ctx, map[string]int64{slacktest.PeerID: 0})
	if err != nil {
		t.Fatalf("edge UsersInfo: %v", err)
	}
	if len(users) != 1 || users[0].Profile.DisplayName != "peer" {
		t.Errorf("edge UsersInfo = %+v; want one user displayed as peer", users)
	}

	// Recording: ordered, with form and raw body.
	reqs := s.Requests()
	if len(reqs) == 0 || reqs[0].Path != "/api/auth.test" {
		t.Errorf("first recorded request = %+v; want /api/auth.test", reqs)
	}
	bootReqs := s.RequestsTo("/api/client.userBoot")
	if len(bootReqs) != 1 || bootReqs[0].Form.Get("token") != "xoxc-test" {
		t.Errorf("client.userBoot requests = %+v; want one carrying the xoxc token", bootReqs)
	}
	edgeReqs := s.RequestsTo("/cache/" + slacktest.TeamID + "/users/info")
	if len(edgeReqs) != 1 || !strings.Contains(string(edgeReqs[0].Body), slacktest.PeerID) {
		t.Errorf("edge users/info requests = %+v; want one whose body names %s", edgeReqs, slacktest.PeerID)
	}

	// (c) The event WebSocket: an injected frame reaches the handler,
	// and a frame the client sends is observable.
	h := &harnessHandler{texts: make(chan string, 1)}
	if err := c.StartWebSocket(h); err != nil {
		t.Fatalf("StartWebSocket: %v", err)
	}
	defer func() {
		_ = c.StopWebSocket()
		<-c.WsDone()
	}()

	s.InjectEvent(`{"type":"message","channel":"` + slacktest.ChannelID + `","user":"` + slacktest.PeerID + `","ts":"1700000009.000100","text":"injected over the fake socket"}`)
	select {
	case got := <-h.texts:
		if got != "injected over the fake socket" {
			t.Errorf("OnMessage text = %q; want the injected frame's text", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("injected event never reached the EventHandler")
	}

	if err := c.SendTyping(slacktest.ChannelID); err != nil {
		t.Fatalf("SendTyping: %v", err)
	}
	frame := string(s.NextClientFrame())
	if !strings.Contains(frame, `"type":"typing"`) || !strings.Contains(frame, slacktest.ChannelID) {
		t.Errorf("client frame = %s; want a typing frame for %s", frame, slacktest.ChannelID)
	}
}
