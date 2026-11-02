package cache

import (
	"database/sql"
	"errors"
	"testing"
)

// threadRow reads one thread_subscriptions row directly; ok is false
// when there is none.
func threadRow(t *testing.T, db *DB, ws, ch, ts string) (lastRead, latestReply string, active, ok bool) {
	t.Helper()
	var activeInt int
	err := db.conn.QueryRow(
		`SELECT last_read, latest_reply, active FROM thread_subscriptions WHERE workspace_id=? AND channel_id=? AND thread_ts=?`,
		ws, ch, ts,
	).Scan(&lastRead, &latestReply, &activeInt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, false
	}
	if err != nil {
		t.Fatalf("reading thread_subscriptions row: %v", err)
	}
	return lastRead, latestReply, activeInt == 1, true
}

func mustUpsert(t *testing.T, db *DB, ws, ch, ts, lastRead string, active bool) {
	t.Helper()
	if err := db.UpsertThreadSubscription(ws, ch, ts, lastRead, active); err != nil {
		t.Fatalf("UpsertThreadSubscription: %v", err)
	}
}

// seedLatestReply sets a row's latest_reply, a column slk's older
// getView sweeps wrote and nothing writes now.
func seedLatestReply(t *testing.T, db *DB, ws, ch, ts, latestReply string) {
	t.Helper()
	if _, err := db.conn.Exec(
		`UPDATE thread_subscriptions SET latest_reply=? WHERE workspace_id=? AND channel_id=? AND thread_ts=?`,
		latestReply, ws, ch, ts,
	); err != nil {
		t.Fatalf("seeding latest_reply: %v", err)
	}
}

func TestUpsertThreadSubscription_Insert(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "1700000000.000100", "1700000000.000200", true)
	lastRead, _, active, ok := threadRow(t, db, "T1", "C1", "1700000000.000100")
	if !ok || lastRead != "1700000000.000200" || !active {
		t.Fatalf("row = (%q, active=%v, ok=%v), want the inserted active row", lastRead, active, ok)
	}
}

func TestUpsertThreadSubscription_UpdateBumpsLastRead(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "1700000000.000100", "1700000000.000200", true)
	mustUpsert(t, db, "T1", "C1", "1700000000.000100", "1700000000.000900", true)
	if lastRead, _, _, _ := threadRow(t, db, "T1", "C1", "1700000000.000100"); lastRead != "1700000000.000900" {
		t.Fatalf("LastRead not updated: %s", lastRead)
	}
}

func TestUpsertThreadSubscription_ToggleActive(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "1700000000.000100", "1700000000.000200", true)
	mustUpsert(t, db, "T1", "C1", "1700000000.000100", "1700000000.000200", false)
	if _, _, active, ok := threadRow(t, db, "T1", "C1", "1700000000.000100"); !ok || active {
		t.Fatalf("row = (active=%v, ok=%v), want a kept, inactive row", active, ok)
	}
}

func TestUpdateThreadLastRead_PreservesTombstone(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "P1", "R1", false)
	if err := db.UpdateThreadLastRead("T1", "C1", "P1", "R9"); err != nil {
		t.Fatalf("UpdateThreadLastRead: %v", err)
	}
	lastRead, _, active, _ := threadRow(t, db, "T1", "C1", "P1")
	if active {
		t.Fatal("UpdateThreadLastRead must not resurrect a tombstoned row")
	}
	if lastRead != "R9" {
		t.Errorf("last_read = %q, want R9", lastRead)
	}
}

func TestUpdateThreadLastRead_PreservesActive(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "P1", "R1", true)
	if err := db.UpdateThreadLastRead("T1", "C1", "P1", "R9"); err != nil {
		t.Fatalf("UpdateThreadLastRead: %v", err)
	}
	if lastRead, _, active, _ := threadRow(t, db, "T1", "C1", "P1"); !active || lastRead != "R9" {
		t.Errorf("row = (%q, active=%v), want (R9, active)", lastRead, active)
	}
}

func TestUpdateThreadLastRead_InsertsActiveRow(t *testing.T) {
	db := setupDBWithWorkspace(t)
	// No prior row: callers insert only for threads Slack says the
	// user follows, so an inserted row starts active.
	if err := db.UpdateThreadLastRead("T1", "C1", "P1", "R5"); err != nil {
		t.Fatalf("UpdateThreadLastRead: %v", err)
	}
	if lastRead, _, active, ok := threadRow(t, db, "T1", "C1", "P1"); !ok || !active || lastRead != "R5" {
		t.Fatalf("row = (%q, active=%v, ok=%v), want an active row with R5", lastRead, active, ok)
	}
}

func TestGetThreadLastRead_MissingRowReturnsEmpty(t *testing.T) {
	db := setupDBWithWorkspace(t)
	got, err := db.GetThreadLastRead("T1", "C1", "P1")
	if err != nil {
		t.Fatalf("GetThreadLastRead on missing row: %v", err)
	}
	if got != "" {
		t.Errorf("GetThreadLastRead = %q, want empty", got)
	}
}

func TestUpdateThreadLastRead_RejectsEmptyKey(t *testing.T) {
	db := setupDBWithWorkspace(t)
	if err := db.UpdateThreadLastRead("", "C1", "P1", "R5"); err == nil {
		t.Error("want error for empty workspaceID, got nil")
	}
	if err := db.UpdateThreadLastRead("T1", "", "P1", "R5"); err == nil {
		t.Error("want error for empty channelID, got nil")
	}
	if err := db.UpdateThreadLastRead("T1", "C1", "", "R5"); err == nil {
		t.Error("want error for empty threadTS, got nil")
	}
}

// A read cursor update must not clobber latest_reply.
func TestUpdateThreadLastRead_PreservesLatestReply(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "1700000100.000000", "1700000150.000000", true)
	seedLatestReply(t, db, "T1", "C1", "1700000100.000000", "1700000200.000000")
	if err := db.UpdateThreadLastRead("T1", "C1", "1700000100.000000", "1700000200.000000"); err != nil {
		t.Fatalf("UpdateThreadLastRead: %v", err)
	}
	if _, latestReply, _, _ := threadRow(t, db, "T1", "C1", "1700000100.000000"); latestReply != "1700000200.000000" {
		t.Errorf("latest_reply = %q, want it preserved", latestReply)
	}
}

// The local mark path fires for ANY thread the user opens, including
// threads opened from the messages pane that they were never subscribed
// to, and must not fabricate a row for them.
func TestUpdateThreadLastReadIfExists_MissingRowIsNotCreated(t *testing.T) {
	db := setupDBWithWorkspace(t)
	if err := db.UpdateThreadLastReadIfExists("T1", "C1", "P1", "R5"); err != nil {
		t.Fatalf("a missing row must not be an error: %v", err)
	}
	if _, _, _, ok := threadRow(t, db, "T1", "C1", "P1"); ok {
		t.Fatal("must not create a row")
	}
}

func TestUpdateThreadLastReadIfExists_AdvancesExistingRow(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "P1", "R1", true)
	if err := db.UpdateThreadLastReadIfExists("T1", "C1", "P1", "R9"); err != nil {
		t.Fatalf("UpdateThreadLastReadIfExists: %v", err)
	}
	if lastRead, _, active, _ := threadRow(t, db, "T1", "C1", "P1"); !active || lastRead != "R9" {
		t.Errorf("row = (%q, active=%v), want (R9, active)", lastRead, active)
	}
}

// `active` is owned by thread_subscribed and thread_unsubscribed.
// Advancing a cursor must neither resurrect a tombstone nor refuse to
// record the read.
func TestUpdateThreadLastReadIfExists_PreservesTombstone(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "P1", "R1", false)
	if err := db.UpdateThreadLastReadIfExists("T1", "C1", "P1", "R9"); err != nil {
		t.Fatalf("UpdateThreadLastReadIfExists: %v", err)
	}
	lastRead, _, active, _ := threadRow(t, db, "T1", "C1", "P1")
	if active {
		t.Fatal("must not resurrect a tombstoned row")
	}
	if lastRead != "R9" {
		t.Errorf("last_read = %q, want the cursor still advanced to R9", lastRead)
	}
}

func TestUpdateThreadLastReadIfExists_PreservesLatestReply(t *testing.T) {
	db := setupDBWithWorkspace(t)
	mustUpsert(t, db, "T1", "C1", "1700000100.000000", "1700000150.000000", true)
	seedLatestReply(t, db, "T1", "C1", "1700000100.000000", "1700000200.000000")
	if err := db.UpdateThreadLastReadIfExists("T1", "C1", "1700000100.000000", "1700000200.000000"); err != nil {
		t.Fatalf("UpdateThreadLastReadIfExists: %v", err)
	}
	if _, latestReply, _, _ := threadRow(t, db, "T1", "C1", "1700000100.000000"); latestReply != "1700000200.000000" {
		t.Errorf("latest_reply = %q, want it preserved", latestReply)
	}
}

func TestUpdateThreadLastReadIfExists_RejectsEmptyKey(t *testing.T) {
	db := setupDBWithWorkspace(t)
	if err := db.UpdateThreadLastReadIfExists("", "C1", "P1", "R5"); err == nil {
		t.Error("want error for empty workspaceID, got nil")
	}
	if err := db.UpdateThreadLastReadIfExists("T1", "", "P1", "R5"); err == nil {
		t.Error("want error for empty channelID, got nil")
	}
	if err := db.UpdateThreadLastReadIfExists("T1", "C1", "", "R5"); err == nil {
		t.Error("want error for empty threadTS, got nil")
	}
}
