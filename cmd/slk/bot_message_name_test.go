package main

import (
	"encoding/json"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui"
	"github.com/slack-go/slack"
)

// One bot posts under a different username per message (chat.postMessage's
// username override), and Slack shows each message's own. The store holds
// one name per bot ID, the first username slk saw it post under, which
// must not stand in for another message's own username.
const (
	botStoredName = "Claude [reviewing loki fail-fast PR]"
	botOwnName    = "Claude [triaging sim env issue 86]"
)

func botNameStore() map[string]string { return map[string]string{"B1": botStoredName} }

// A bot message cached by a history or thread fetch (keyed on its bot_id,
// raw_json as Slack sent it) reloads under its own username: the thread
// panel's parent and the cache-first render of a channel come from here.
func TestCachedBotMessageKeepsItsOwnUsername(t *testing.T) {
	db := newTestDB(t)
	raw, _ := json.Marshal(slack.Message{Msg: slack.Msg{BotID: "B1", Username: botOwnName, Text: "please review", Timestamp: "1.000001"}})
	row := cache.Message{TS: "1.000001", ChannelID: "C1", UserID: "B1", Text: "please review", RawJSON: string(raw)}

	item := enrichCachedRow(db, "USELF", "C1", row, newUserNameFill(newUserNameStore(botNameStore())), "3:04 PM", "test", nil, nil)

	if item.UserName != botOwnName {
		t.Errorf("UserName = %q, want the message's own username %q", item.UserName, botOwnName)
	}
}

// A bot message arriving over the WebSocket shows its own username, live
// and again when it is reloaded from the row the handler cached.
func TestLiveBotMessageKeepsItsOwnUsername(t *testing.T) {
	db := newTestDB(t)
	_ = db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "gitea", Type: "channel"})
	var got []ui.NewMessageMsg
	h := &rtmEventHandler{
		db:              db,
		workspaceID:     "T1",
		wsCtx:           &WorkspaceContext{},
		userNames:       newUserNameStore(botNameStore()),
		isActive:        func() bool { return true },
		activeChannelID: func() string { return "C1" },
		program: sendFunc(func(msg tea.Msg) {
			if m, ok := msg.(ui.NewMessageMsg); ok {
				got = append(got, m)
			}
		}),
	}
	h.OnMessage("C1", "", "2.000001", "Reviewing #3522", "1.000001", "", false, false, nil, slack.Blocks{}, nil, "B1", botOwnName)

	if len(got) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(got))
	}
	if got[0].Message.UserName != botOwnName {
		t.Errorf("live UserName = %q, want the message's own username %q", got[0].Message.UserName, botOwnName)
	}

	rows, err := db.GetThreadReplies("C1", "1.000001")
	if err != nil || len(rows) != 1 {
		t.Fatalf("cached rows = %d (err %v), want the one reply", len(rows), err)
	}
	item := enrichCachedRow(db, "USELF", "C1", rows[0], newUserNameFill(h.userNames), "3:04 PM", "test", nil, nil)
	if item.UserName != botOwnName {
		t.Errorf("reloaded UserName = %q, want the message's own username %q", item.UserName, botOwnName)
	}
}

// A thread reply from a bot in a background workspace reaches the
// tracked agent thread under its own username too.
func TestBackgroundBotReplyKeepsItsOwnUsername(t *testing.T) {
	var got []ui.NewMessageMsg
	h := &rtmEventHandler{
		workspaceID:     "T1",
		wsCtx:           &WorkspaceContext{},
		userNames:       newUserNameStore(botNameStore()),
		isActive:        func() bool { return false },
		activeChannelID: func() string { return "" },
		program: sendFunc(func(msg tea.Msg) {
			if m, ok := msg.(ui.NewMessageMsg); ok {
				got = append(got, m)
			}
		}),
	}
	h.OnMessage("C1", "", "2.000001", "Reviewing #3522", "1.000001", "", false, false, nil, slack.Blocks{}, nil, "B1", botOwnName)

	if len(got) != 1 {
		t.Fatalf("dispatched %d messages, want 1", len(got))
	}
	if got[0].Message.UserName != botOwnName {
		t.Errorf("UserName = %q, want the message's own username %q", got[0].Message.UserName, botOwnName)
	}
}

// A bot message without a username of its own shows the bot's stored
// name, from the cache and live.
func TestBotMessageWithoutUsernameShowsStoredName(t *testing.T) {
	db := newTestDB(t)
	raw, _ := json.Marshal(slack.Message{Msg: slack.Msg{BotID: "B1", Text: "beep", Timestamp: "1.000001"}})
	row := cache.Message{TS: "1.000001", ChannelID: "C1", UserID: "B1", Text: "beep", RawJSON: string(raw)}
	item := enrichCachedRow(db, "USELF", "C1", row, newUserNameFill(newUserNameStore(botNameStore())), "3:04 PM", "test", nil, nil)
	if item.UserName != botStoredName {
		t.Errorf("cached UserName = %q, want the stored name %q", item.UserName, botStoredName)
	}

	var got []ui.NewMessageMsg
	h := &rtmEventHandler{
		wsCtx:     &WorkspaceContext{},
		userNames: newUserNameStore(botNameStore()),
		program: sendFunc(func(msg tea.Msg) {
			if m, ok := msg.(ui.NewMessageMsg); ok {
				got = append(got, m)
			}
		}),
	}
	h.OnMessage("C1", "", "2.000001", "beep", "", "", false, false, nil, slack.Blocks{}, nil, "B1", "")
	if len(got) != 1 || got[0].Message.UserName != botStoredName {
		t.Errorf("live dispatch = %+v, want one message named %q", got, botStoredName)
	}
}
