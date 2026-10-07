package main

import (
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/usernames"
)

// An ephemeral reaches the UI flagged, named by its bot, and leaves
// SQLite as it was: no message row, no latest_synced_ts, no unread. The
// same message delivered as an ordinary one writes all three, which is
// what the assertions would catch.
func TestOnEphemeralMessage_DispatchesWithoutPersisting(t *testing.T) {
	db := newTestDB(t)
	seedBootChannel(t, db, "C1")
	sender := &captureSender{}
	h := &rtmEventHandler{
		program:      sender,
		workspaceID:  "T1",
		db:           db,
		userNames:    usernames.FromMap(nil),
		isActive:     func() bool { return true },
		channelTypes: map[string]string{"C1": "channel"},
	}
	persisted := func() (row bool, latest string, unread bool) {
		_, err := db.GetMessage("C1", "1787400000.000200")
		rs, _ := db.GetChannelReadState("C1")
		return err == nil, db.GetChannelLatestSyncedTS("C1"), rs.HasUnread
	}

	h.OnEphemeralMessage("C1", "", "1787400000.000200", "Your annotation was sent.", "1787300000.000100", "bot_message", nil, slack.Blocks{}, nil, "B1", "Colony")

	if row, latest, unread := persisted(); row || latest != "" || unread {
		t.Errorf("ephemeral persisted: message row %v, latest_synced_ts %q, has_unread %v", row, latest, unread)
	}
	msgs := sentOfType[ui.NewMessageMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 NewMessageMsg, got %d", len(msgs))
	}
	m := msgs[0]
	if !m.Message.IsEphemeral || m.TeamID != "T1" || m.ChannelID != "C1" || m.Message.ThreadTS != "1787300000.000100" || m.Message.UserName != "Colony" {
		t.Errorf("NewMessageMsg = %+v, want an ephemeral reply in C1 by Colony", m)
	}

	h.OnMessage("C1", "", "1787400000.000200", "Your annotation was sent.", "", "bot_message", false, nil, slack.Blocks{}, nil, "B1", "Colony")
	if row, latest, unread := persisted(); !row || latest == "" || !unread {
		t.Errorf("control: OnMessage wrote message row %v, latest_synced_ts %q, has_unread %v; want all three", row, latest, unread)
	}
}
