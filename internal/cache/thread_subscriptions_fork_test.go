package cache

import (
	"testing"
)

func TestThreadNewestActivity(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	seedMsg := func(ts, threadTS string, deleted bool) {
		t.Helper()
		if err := db.UpsertMessage(Message{
			TS: ts, ChannelID: "C1", WorkspaceID: "T1", UserID: "U1",
			Text: "m", ThreadTS: threadTS, IsDeleted: deleted,
		}); err != nil {
			t.Fatalf("UpsertMessage %s: %v", ts, err)
		}
	}

	// Entirely unknown thread: no subscription row, no messages.
	got, err := db.ThreadNewestActivity("T1", "C1", "100.000000")
	if err != nil {
		t.Fatalf("ThreadNewestActivity: %v", err)
	}
	if got != "" {
		t.Fatalf("unknown thread: want \"\", got %q", got)
	}

	// Subscription row without latest_reply (live upsert path): still unknown.
	mustUpsert(t, db, "T1", "C1", "100.000000", "100.000000", true)
	if got, _ = db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "" {
		t.Fatalf("row without latest_reply: want \"\", got %q", got)
	}

	// Parent row only (cached with empty thread_ts, ts == thread_ts).
	seedMsg("100.000000", "", false)
	if got, _ = db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "100.000000" {
		t.Fatalf("parent-only: want parent ts, got %q", got)
	}

	// Cached replies: newest wins.
	seedMsg("101.000000", "100.000000", false)
	seedMsg("102.000000", "100.000000", false)
	if got, _ = db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "102.000000" {
		t.Fatalf("cached replies: want newest reply, got %q", got)
	}

	// Deleted newest reply doesn't count.
	seedMsg("103.000000", "100.000000", true)
	if got, _ = db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "102.000000" {
		t.Fatalf("deleted reply counted: got %q", got)
	}

	// getView watermark beyond the cache wins (the boot case: replies
	// not fetched yet).
	fresh := []ThreadSubscription{{
		WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "100.000000",
		LastRead: "100.000000", LatestReply: "105.000000", Active: true,
	}}
	if err := db.ReconcileThreadSubscriptions("T1", fresh); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if got, _ = db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "105.000000" {
		t.Fatalf("latest_reply beyond cache: want 105.000000, got %q", got)
	}
}

func TestDeleteMessage_RetractsLatestReplyWatermark(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	seed := func(ts string) {
		t.Helper()
		if err := db.UpsertMessage(Message{
			TS: ts, ChannelID: "C1", WorkspaceID: "T1", UserID: "U2",
			Text: "m", ThreadTS: "100.000000",
		}); err != nil {
			t.Fatalf("UpsertMessage %s: %v", ts, err)
		}
	}
	seed("101.000000")
	seed("102.000000")
	fresh := []ThreadSubscription{{
		WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "100.000000",
		LastRead: "101.000000", LatestReply: "102.000000", Active: true,
	}}
	if err := db.ReconcileThreadSubscriptions("T1", fresh); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	// Deleting the message the watermark points at must retract it:
	// otherwise a read-to-end at the surviving newest reply (101)
	// computes 101 < 102 and renders a fully-read thread unread until
	// the next getView reconcile.
	if err := db.DeleteMessage("C1", "102.000000"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	got, err := db.ThreadNewestActivity("T1", "C1", "100.000000")
	if err != nil {
		t.Fatalf("ThreadNewestActivity: %v", err)
	}
	if got != "101.000000" {
		t.Fatalf("newest activity after deleting the watermark message: want surviving reply 101.000000, got %q", got)
	}

	// Deleting a non-watermark message leaves the watermark alone.
	seed("103.000000")
	fresh[0].LatestReply = "103.000000"
	if err := db.ReconcileThreadSubscriptions("T1", fresh); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if err := db.DeleteMessage("C1", "101.000000"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	if got, _ := db.ThreadNewestActivity("T1", "C1", "100.000000"); got != "103.000000" {
		t.Fatalf("deleting a non-watermark message must not retract: want 103.000000, got %q", got)
	}
}
