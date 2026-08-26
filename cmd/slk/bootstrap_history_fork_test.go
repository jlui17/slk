package main

import (
	"encoding/json"
	"testing"

	"github.com/gammons/slk/internal/bootstrap"
	"github.com/gammons/slk/internal/cache"
)

func seedBootChannel(t *testing.T, db *cache.DB, channelID string) {
	t.Helper()
	if err := db.UpsertChannel(cache.Channel{
		ID:          channelID,
		WorkspaceID: "T1",
		Name:        "general",
		Type:        "channel",
		IsMember:    true,
	}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
}

func rawMsg(t *testing.T, ts, user, text string) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(map[string]any{"ts": ts, "user": user, "text": text, "type": "message"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestPersistBootstrapHistoryCachesAndStamps(t *testing.T) {
	db := newCacheForTest(t)
	seedBootChannel(t, db, "C1")

	persistBootstrapHistory(db, "T1", &bootstrap.Result{
		OpenedChannelID: "C1",
		Messages: []json.RawMessage{
			rawMsg(t, "1700000002.000000", "U2", "second"),
			rawMsg(t, "1700000001.000000", "U1", "first"),
		},
	})

	rows, err := db.GetMessages("C1", 50, "")
	if err != nil {
		t.Fatalf("GetMessages: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("cached %d messages, want 2", len(rows))
	}
	if rows[0].TS != "1700000001.000000" || rows[0].UserID != "U1" || rows[0].Text != "first" {
		t.Errorf("row[0] = %+v, want the boot-fetched first message", rows[0])
	}
	if rows[0].RawJSON == "" {
		t.Error("RawJSON not preserved from the boot response bytes")
	}
	if db.GetChannelSyncedAt("C1") == 0 {
		t.Error("channel not stamped synced: the UI would refetch the history bootstrap just fetched")
	}
}

func TestPersistBootstrapHistoryFailedLoadDoesNotStamp(t *testing.T) {
	db := newCacheForTest(t)
	seedBootChannel(t, db, "C1")

	// Empty Messages + empty UnchangedTS alongside a non-empty
	// OpenedChannelID is bootstrap's "load failed on both paths"
	// shape: the UI must keep its own fetch path.
	persistBootstrapHistory(db, "T1", &bootstrap.Result{OpenedChannelID: "C1"})
	if got := db.GetChannelSyncedAt("C1"); got != 0 {
		t.Errorf("failed boot load stamped synced_at=%d, want 0", got)
	}

	// No channel opened at all: nothing to persist.
	persistBootstrapHistory(db, "T1", &bootstrap.Result{})
	if got := db.GetChannelSyncedAt("C1"); got != 0 {
		t.Errorf("no-channel boot stamped synced_at=%d, want 0", got)
	}
}

func TestPersistBootstrapHistoryUnchangedOnlyStamps(t *testing.T) {
	db := newCacheForTest(t)
	seedBootChannel(t, db, "C1")

	// The incremental history fallback can return zero changed
	// messages while confirming the cached ones current — that is a
	// successful sync, not a failure.
	persistBootstrapHistory(db, "T1", &bootstrap.Result{
		OpenedChannelID: "C1",
		UnchangedTS:     []string{"1700000001.000000"},
	})
	if db.GetChannelSyncedAt("C1") == 0 {
		t.Error("unchanged-confirmed boot did not stamp synced_at; the UI would refetch a cache the server just vouched for")
	}
}
