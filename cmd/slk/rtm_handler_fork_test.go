package main

import (
	"slices"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/cache"
	slackclient "github.com/gammons/slk/internal/slack"
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

	h.OnEphemeralMessage(slackclient.EphemeralMessage{ChannelID: "C1", BotID: "B1", Username: "Colony", TS: "1787400000.000200", ThreadTS: "1787300000.000100", Subtype: "bot_message", Text: "Your annotation was sent."})

	if row, latest, unread := persisted(); row || latest != "" || unread {
		t.Errorf("ephemeral persisted: message row %v, latest_synced_ts %q, has_unread %v", row, latest, unread)
	}
	msgs := sentOfType[ui.NewMessageMsg](sender)
	if len(msgs) != 1 {
		t.Fatalf("want 1 NewMessageMsg, got %d", len(msgs))
	}
	m := msgs[0]
	if !m.Message.Ephemeral || m.TeamID != "T1" || m.ChannelID != "C1" || m.Message.ThreadTS != "1787300000.000100" || m.Message.UserName != "Colony" {
		t.Errorf("NewMessageMsg = %+v, want an ephemeral reply in C1 by Colony", m)
	}

	h.OnMessage("C1", "", "1787400000.000200", "Your annotation was sent.", "", "bot_message", false, false, nil, slack.Blocks{}, nil, "B1", "Colony")
	if row, latest, unread := persisted(); !row || latest == "" || !unread {
		t.Errorf("control: OnMessage wrote message row %v, latest_synced_ts %q, has_unread %v; want all three", row, latest, unread)
	}
}

// A background workspace's reactions reach the UI only for the tracked
// agent thread's ack, which only a bot gives; each one sent costs a full
// Update and View. The active workspace's reach it whoever reacts.
func TestReactionsReachUIFromBackgroundWorkspaceOnlyForBots(t *testing.T) {
	for _, tc := range []struct {
		name     string
		active   bool
		wantFrom []string
	}{
		{name: "background", active: false, wantFrom: []string{"UBOT"}},
		{name: "active", active: true, wantFrom: []string{"UHUMAN", "UBOT"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t)
			for _, u := range []cache.User{{ID: "UBOT", WorkspaceID: "T1", Name: "claude", IsBot: true}, {ID: "UHUMAN", WorkspaceID: "T1", Name: "justin"}} {
				if err := db.UpsertUser(u); err != nil {
					t.Fatal(err)
				}
			}
			sender := &captureSender{}
			active := tc.active
			h := &rtmEventHandler{program: sender, workspaceID: "T1", db: db, isActive: func() bool { return active }}

			for _, user := range []string{"UHUMAN", "UBOT"} {
				h.OnReactionAdded("C1", "1.0", user, "eyes")
				h.OnReactionRemoved("C1", "1.0", user, "eyes")
			}

			var added, removed []string
			for _, m := range sentOfType[ui.ReactionAddedMsg](sender) {
				added = append(added, m.UserID)
			}
			for _, m := range sentOfType[ui.ReactionRemovedMsg](sender) {
				removed = append(removed, m.UserID)
			}
			if !slices.Equal(added, tc.wantFrom) || !slices.Equal(removed, tc.wantFrom) {
				t.Errorf("reactions sent to the UI from added %v, removed %v; want %v for both", added, removed, tc.wantFrom)
			}
		})
	}
}

// Every slk running on the machine shares one cache and gets every
// reaction event, so the cache sees each event once per slk. The bot's
// ack (:eyes: added on start, removed when done, added again) must still
// leave one entry for the bot, as Slack shows: "eyes ×1".
func TestOnReaction_CacheCountsEachUserOnce(t *testing.T) {
	const ch, ts, bot = "C1", "1.0", "UBOT"
	db := newTestDB(t)
	panes := []*rtmEventHandler{
		{program: &captureSender{}, workspaceID: "T1", db: db},
		{program: &captureSender{}, workspaceID: "T1", db: db},
	}
	added := func() {
		for _, h := range panes {
			h.OnReactionAdded(ch, ts, bot, "eyes")
		}
	}
	removed := func() {
		for _, h := range panes {
			h.OnReactionRemoved(ch, ts, bot, "eyes")
		}
	}
	wantEyes := func(step string, users []string, count int) {
		t.Helper()
		rows, err := db.GetReactions(ts, ch)
		if err != nil {
			t.Fatalf("GetReactions: %v", err)
		}
		if len(users) == 0 {
			if len(rows) != 0 {
				t.Fatalf("after %s: cached %+v, want no reaction", step, rows)
			}
			return
		}
		if len(rows) != 1 || !slices.Equal(rows[0].UserIDs, users) || rows[0].Count != count {
			t.Fatalf("after %s: cached %+v, want eyes %v ×%d", step, rows, users, count)
		}
	}

	added()
	wantEyes("add", []string{bot}, 1)
	removed()
	wantEyes("add, remove", nil, 0)
	added()
	removed()
	added()
	panes[0].OnReactionAdded(ch, ts, bot, "eyes") // a duplicate delivery
	wantEyes("add, remove, add, remove, add, add", []string{bot}, 1)

	// A row an earlier build stored once per slk mends on the next event.
	if err := db.UpsertReaction(ts, ch, "eyes", []string{bot, bot, bot, "U2"}, 4); err != nil {
		t.Fatalf("UpsertReaction: %v", err)
	}
	added()
	wantEyes("add over a row stored once per slk", []string{bot, "U2"}, 2)
	removed()
	wantEyes("remove", []string{"U2"}, 1)
}
