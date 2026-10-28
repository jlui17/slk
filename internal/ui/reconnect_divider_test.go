package ui

import (
	"strings"
	"testing"
)

// The "── new ──" line stays where it was while the channel is on screen,
// as in Slack, when a websocket reconnect refreshes the channel. By then
// opening the channel has marked it read to its newest message, so the
// refresh reports that as the last read; taking it would drop the line.
func TestReconnectRefreshKeepsNewDivider(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "eng", Type: "channel"})
	// Opening the channel: read up to 1.0, so 2.0 and 3.0 are new.
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: testMessageItems(3), LastReadTS: "1.0", MarkedTS: "3.0"})
	if !strings.Contains(a.messagepane.View(40, 80), "── new ──") {
		t.Fatal("precondition: the open channel draws the new line")
	}

	// The refresh rtmEventHandler.refreshChannel sends after a reconnect:
	// 4.0 arrived while the channel was on screen and was marked read too.
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: testMessageItems(4), LastReadTS: "4.0", KeepLastRead: true})

	if got := a.messagepane.LastReadTS(); got != "1.0" {
		t.Errorf("LastReadTS = %q after the reconnect refresh, want 1.0 kept", got)
	}
	if !strings.Contains(a.messagepane.View(40, 80), "── new ──") {
		t.Error("the new line is gone after the reconnect refresh")
	}
}

// A pane with no last-read boundary yet takes the refresh's.
func TestReconnectRefreshSetsMissingLastRead(t *testing.T) {
	a := NewApp()
	_, _ = a.Update(ChannelSelectedMsg{ID: "C1", Name: "eng", Type: "channel"})
	_, _ = a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: testMessageItems(3), KeepLastRead: true, LastReadTS: "2.0"})

	if got := a.messagepane.LastReadTS(); got != "2.0" {
		t.Errorf("LastReadTS = %q, want the refresh's 2.0", got)
	}
}
