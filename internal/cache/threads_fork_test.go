package cache

import (
	"testing"
)

func TestListSubscribedThreads_ParentOnlyMarkUnread(t *testing.T) {
	const selfID = "U1"
	db := setupDBWithWorkspace(t)
	if err := db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	// A replyless thread's parent is cached with empty thread_ts (it
	// only gains one once replies exist). A remote mark-unread sets
	// last_read 1µs below the parent; the row must render unread —
	// previously the cached-max subquery matched replies only, so
	// effLatest fell back to last_read and the reload silently undid
	// the user's mark.
	mustUpsertMsg(t, db, "1700000100.000000", "C1", "U2", "parent", "")
	if err := db.UpsertThreadSubscription("T1", "C1", "1700000100.000000", "1700000099.999999", true); err != nil {
		t.Fatalf("UpsertThreadSubscription: %v", err)
	}

	got, err := db.ListSubscribedThreads("T1", selfID)
	if err != nil {
		t.Fatalf("ListSubscribedThreads: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("want 1, got %d", len(got))
	}
	if !got[0].Unread {
		t.Fatalf("expected Unread=true for a parent-only thread marked unread (last_read < parent ts)")
	}
	if got[0].Unread && got[0].LastReplyTS != "1700000100.000000" {
		t.Fatalf("LastReplyTS should be the parent ts, got %q", got[0].LastReplyTS)
	}
}

func TestListSubscribedThreads_ParentOnlyRead(t *testing.T) {
	const selfID = "U1"
	db := setupDBWithWorkspace(t)
	if err := db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	// Companion boundary: last_read == parent ts is caught up.
	mustUpsertMsg(t, db, "1700000100.000000", "C1", "U2", "parent", "")
	if err := db.UpsertThreadSubscription("T1", "C1", "1700000100.000000", "1700000100.000000", true); err != nil {
		t.Fatalf("UpsertThreadSubscription: %v", err)
	}

	got, err := db.ListSubscribedThreads("T1", selfID)
	if err != nil {
		t.Fatalf("ListSubscribedThreads: %v", err)
	}
	if len(got) != 1 || got[0].Unread {
		t.Fatalf("expected one read row for last_read == parent ts, got %+v", got)
	}
}

func TestListSubscribedThreads_ParentOnlySelfRootSuppressed(t *testing.T) {
	const selfID = "U1"
	db := setupDBWithWorkspace(t)
	if err := db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel"}); err != nil {
		t.Fatalf("UpsertChannel: %v", err)
	}
	// A self-authored root with a lagging last_read (e.g. the zero
	// sentinel on a fresh subscription) must not flag your own
	// replyless thread unread — the same just-sent suppression replies
	// get, now that the parent row counts as newest activity.
	mustUpsertMsg(t, db, "1700000100.000000", "C1", selfID, "my root", "")
	if err := db.UpsertThreadSubscription("T1", "C1", "1700000100.000000", "0000000000.000000", true); err != nil {
		t.Fatalf("UpsertThreadSubscription: %v", err)
	}

	got, err := db.ListSubscribedThreads("T1", selfID)
	if err != nil {
		t.Fatalf("ListSubscribedThreads: %v", err)
	}
	if len(got) != 1 || got[0].Unread {
		t.Fatalf("expected self-authored parent-only thread suppressed (read), got %+v", got)
	}
}
