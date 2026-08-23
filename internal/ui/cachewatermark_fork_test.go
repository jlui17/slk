package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// newWatermarkTestApp wires an App whose channel service reports a
// tunable synced_at and counts background fetches.
func newWatermarkTestApp(t *testing.T, syncedAt *int64, fetches *int) *App {
	t.Helper()
	a := NewApp()
	a.width = 120
	a.height = 30
	a.activeTeamID = "T1"
	a.SetChannelService(NewChannelService(ChannelServiceFuncs{
		ReadCache: func(ids.ChannelID) []messages.MessageItem {
			return []messages.MessageItem{{TS: "1.0", UserName: "alice", UserID: "U1", Text: "hi", Timestamp: "1:00 PM"}}
		},
		SyncedAt: func(ids.ChannelID) int64 { return *syncedAt },
		Fetch:    func(ids.ChannelID, string) tea.Msg { *fetches++; return nil },
		MarkRead: func(ids.ChannelID, ids.MessageTS) tea.Msg { return nil },
	}))
	_ = a.View()
	return a
}

func selectChannel(a *App, id string) {
	_, cmd := a.Update(ChannelSelectedMsg{ID: id, Name: "general", Type: "channel"})
	_ = drainBatch(cmd)
}

func TestChannelOpen_WatermarkDemotesProvablyFreshCache(t *testing.T) {
	syncedAt := time.Now().Unix()
	fetches := 0
	a := newWatermarkTestApp(t, &syncedAt, &fetches)

	selectChannel(a, "C1")
	if fetches != 0 {
		t.Fatalf("fresh cache with no watermark fetched %d times; want tier-1 (no fetch)", fetches)
	}

	_, _ = a.Update(CacheStaleBeforeMsg{WorkspaceID: "T1", Before: time.Unix(syncedAt, 0).Add(2 * time.Second)})
	selectChannel(a, "C1")
	if fetches != 1 {
		t.Fatalf("cache synced before the reconnect watermark fetched %d times; want tier-2 (one background fetch)", fetches)
	}

	syncedAt = time.Now().Unix() + 5
	selectChannel(a, "C1")
	if fetches != 1 {
		t.Fatalf("cache synced after the watermark fetched %d more times; want tier-1 again (total 1, got %d)", fetches-1, fetches)
	}
}

func TestChannelOpen_WatermarkOfOtherWorkspaceIsIgnored(t *testing.T) {
	syncedAt := time.Now().Unix()
	fetches := 0
	a := newWatermarkTestApp(t, &syncedAt, &fetches)

	_, _ = a.Update(CacheStaleBeforeMsg{WorkspaceID: "T2", Before: time.Unix(syncedAt, 0).Add(2 * time.Second)})
	selectChannel(a, "C1")
	if fetches != 0 {
		t.Fatalf("another workspace's watermark demoted this workspace's fresh cache (%d fetches); want 0", fetches)
	}
}

func TestCacheWatermark_NeverMovesBackward(t *testing.T) {
	syncedAt := time.Now().Unix()
	fetches := 0
	a := newWatermarkTestApp(t, &syncedAt, &fetches)

	later := time.Unix(syncedAt, 0).Add(2 * time.Second)
	_, _ = a.Update(CacheStaleBeforeMsg{WorkspaceID: "T1", Before: later})
	_, _ = a.Update(CacheStaleBeforeMsg{WorkspaceID: "T1", Before: later.Add(-time.Minute)})
	selectChannel(a, "C1")
	if fetches != 1 {
		t.Fatalf("an older watermark rewound the newer one (%d fetches); want the newest watermark to hold (1 fetch)", fetches)
	}
}
