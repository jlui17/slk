// cmd/slk/wctx_races_test.go
//
// Regression guards for data races on WorkspaceContext's shared
// fields, in the production call shapes that raced before the fields
// were made goroutine-safe. These tests MUST run under -race to mean
// anything; do not delete them as slow.
package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/sharedmap"
	"github.com/gammons/slk/internal/usernames"
	"github.com/slack-go/slack"
)

func imChannel(id, user string) slack.Channel {
	return slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID:   id,
				IsIM: true,
				User: user,
			},
		},
	}
}

// The DM-sweep goroutine's write shape (resolveDMNames) against the WS
// goroutine's read shape (OnConversationOpened → buildChannelItem).
// Before BotUserIDs was store-backed this was a fatal concurrent-map
// throw.
func TestBotUserIDsConcurrentSweepWriteAndOpenRead(t *testing.T) {
	wctx := &WorkspaceContext{
		BotUserIDs: sharedmap.New[string, bool](),
		UserNames:  usernames.NewStore(),
	}
	cfg := config.Config{}
	const n = 500
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			wctx.BotUserIDs.Set(fmt.Sprintf("U%04d", i), true)
		}
	}()
	for {
		select {
		case <-done:
			item, _ := buildChannelItem(imChannel("D0007", "U0007"), wctx, cfg, "T1")
			if item.Type != "app" {
				t.Fatalf("Type = %q, want app (bot flag lost)", item.Type)
			}
			return
		default:
		}
		for i := 0; i < 8; i++ {
			buildChannelItem(imChannel(fmt.Sprintf("D%04d", i), fmt.Sprintf("U%04d", i)), wctx, cfg, "T1")
		}
	}
}

// The UI goroutine's write shape (RecordVisit, fired on every channel
// selection) against the finder cmd goroutine's read shape
// (SearchRemote → searchChannelsRemote → topVisitedChannels, which
// iterates while the request is in flight). Before
// LastVisitedByChannel was store-backed this was a fatal
// concurrent-map throw inside topVisitedChannels' sort.
func TestLastVisitedConcurrentVisitWriteAndSearchRead(t *testing.T) {
	wctx := &WorkspaceContext{LastVisitedByChannel: sharedmap.New[string, int64]()}
	const n = 500
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			wctx.LastVisitedByChannel.Set(fmt.Sprintf("C%04d", i), int64(i))
		}
	}()
	for {
		select {
		case <-done:
			top := topVisitedChannels(wctx.LastVisitedByChannel.Current(), maxTopChannels)
			if len(top) == 0 || top[0] != fmt.Sprintf("C%04d", n-1) {
				t.Fatalf("topVisitedChannels = %v, want most-recent C%04d first", top, n-1)
			}
			return
		default:
		}
		_ = topVisitedChannels(wctx.LastVisitedByChannel.Current(), maxTopChannels)
	}
}

// The emoji-fetch goroutine's publish shape against the
// workspace-switch cmd goroutine's read shape (WorkspaceSwitchedMsg
// construction). Before the accessor pair existed this was an
// unsynchronized field rebind.
func TestCustomEmojiConcurrentFetchAndSwitchRead(t *testing.T) {
	wctx := &WorkspaceContext{}
	const n = 500
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			wctx.SetCustomEmoji(map[string]string{"wave": fmt.Sprintf("https://e/%d.png", i)})
		}
	}()
	for {
		select {
		case <-done:
			m := wctx.CustomEmoji()
			if m["wave"] != fmt.Sprintf("https://e/%d.png", n-1) {
				t.Fatalf("CustomEmoji[wave] = %q, want the last published URL", m["wave"])
			}
			return
		default:
		}
		m := wctx.CustomEmoji()
		_ = m["wave"]
	}
}

// The bootstrap goroutine's write shape (bootstrapPresenceAndDND's write
// half, the real applyBootstrappedStatus) against the WS read goroutine's
// handlers (OnSelfPresenceChange, OnDNDChange), which read-modify-send the
// same triple. Before the selfStatus store this was an unsynchronized
// mixed-size race: a torn Presence string or DNDEndTS is crash-capable or
// silently wrong.
func TestSelfStatusConcurrentBootstrapApplyAndWSEvents(t *testing.T) {
	wctx := &WorkspaceContext{TeamID: "T1", UserID: "U1"}
	h := &rtmEventHandler{program: &captureSender{}, workspaceID: "T1", wsCtx: wctx}
	snoozeEnd := time.Now().Add(time.Hour)
	dnd := &slack.DNDStatus{
		SnoozeInfo: slack.SnoozeInfo{SnoozeEnabled: true, SnoozeEndTime: int(snoozeEnd.Unix())},
	}
	snoozeEndTS := time.Unix(snoozeEnd.Unix(), 0)
	// checkPair asserts the snapshot's DND pair came from ONE write:
	// bootstrap writes (true, snoozeEndTS), OnDNDChange writes (false,
	// zero) — any other combination is a mixed-generation read.
	checkPair := func(st selfStatus) {
		t.Helper()
		if st.DNDEnabled && !st.DNDEndTS.Equal(snoozeEndTS) {
			t.Fatalf("mixed DND pair: enabled with end %v, want %v", st.DNDEndTS, snoozeEndTS)
		}
		if !st.DNDEnabled && !st.DNDEndTS.IsZero() {
			t.Fatalf("mixed DND pair: disabled with non-zero end %v", st.DNDEndTS)
		}
	}
	const n = 500
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < n; i++ {
			tok := wctx.selfStatus.BeginBootstrap()
			applyBootstrappedStatus(wctx, h.program, tok,
				&slack.UserPresence{Presence: "active"}, dnd)
		}
	}()
	for {
		select {
		case <-done:
			checkPair(wctx.selfStatus.Snapshot())
			return
		default:
		}
		h.OnSelfPresenceChange("away")
		h.OnDNDChange(false, 0)
		checkPair(wctx.selfStatus.Snapshot())
	}
}
