package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
)

type historyAnswer struct{}

func (historyAnswer) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"ok":true,"messages":[],"pin_count":2}`)),
		Request:    r,
	}, nil
}

// A history read's pin count reaches the UI under the team auth.test
// named, the key the rtm handler and the router use, not the token's.
func TestNotifyPinCounts_KeysByAuthTestTeam(t *testing.T) {
	client := slackclient.NewTestClient("xoxc-test", "d", slackclient.WithInnerTransport(historyAnswer{}), slackclient.WithTeamID("TAUTH"))
	var got []tea.Msg
	notifyPinCounts(client, func(m tea.Msg) { got = append(got, m) })
	if _, err := client.GetHistory(context.Background(), "C1", 50, ""); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("messages %v, want one PinCountMsg", got)
	}
	if m, ok := got[0].(ui.PinCountMsg); !ok || m.TeamID != "TAUTH" || m.ChannelID != "C1" || m.Count != 2 {
		t.Errorf("got %+v, want C1's count of 2 under TAUTH", got[0])
	}
}
