package cache

import (
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
)

// TestUpsertMessages_MatchesIndividualUpserts pins UpsertMessages to
// N-times-UpsertMessage semantics: the batch duplicates UpsertMessage's
// SQL rather than sharing a const, so this test is what keeps the two
// from drifting. Covers both the insert branch and the on-conflict
// update branch (including created_at surviving the update).
func TestUpsertMessages_MatchesIndividualUpserts(t *testing.T) {
	individual := setupDBWithWorkspace(t)
	defer individual.Close()
	batched := setupDBWithWorkspace(t)
	defer batched.Close()

	seed := Message{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1",
		UserID: "U1", Text: "original", CreatedAt: 100}
	for _, db := range []*DB{individual, batched} {
		if err := db.UpsertMessage(seed); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}

	msgs := []Message{
		// Conflict with the seed: every updatable column changed, plus
		// a created_at the update must NOT take.
		{TS: "1.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2",
			Text: "edited", ThreadTS: "1.000000", ReplyCount: 3,
			EditedAt: "2.000000", IsDeleted: true, RawJSON: `{"a":1}`,
			CreatedAt: 999, Subtype: "thread_broadcast"},
		{TS: "2.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1",
			Text: "plain", CreatedAt: 200},
		{TS: "3.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U3",
			Text: "reply", ThreadTS: "2.000000", CreatedAt: 300, RawJSON: `{"b":2}`},
	}

	for _, m := range msgs {
		if err := individual.UpsertMessage(m); err != nil {
			t.Fatalf("UpsertMessage: %v", err)
		}
	}
	if err := batched.UpsertMessages(msgs); err != nil {
		t.Fatalf("UpsertMessages: %v", err)
	}

	const all = `
		SELECT ts, channel_id, workspace_id, user_id, text, thread_ts, reply_count, edited_at, is_deleted, raw_json, created_at, subtype
		FROM messages ORDER BY ts`
	want, err := individual.queryMessages(all)
	if err != nil {
		t.Fatalf("reading individual rows: %v", err)
	}
	got, err := batched.queryMessages(all)
	if err != nil {
		t.Fatalf("reading batched rows: %v", err)
	}
	if len(want) != 3 {
		t.Fatalf("individual rows = %d, want 3", len(want))
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("batched rows differ from individual upserts:\ngot:  %+v\nwant: %+v", got, want)
	}
	if want[0].CreatedAt != 100 {
		t.Errorf("conflict update overwrote created_at: got %d, want 100", want[0].CreatedAt)
	}
}

func TestUpsertMessages_EmptyBatchIsNoOp(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if err := db.UpsertMessages(nil); err != nil {
		t.Fatalf("UpsertMessages(nil): %v", err)
	}
}

func benchMessages(n int) []Message {
	msgs := make([]Message, 0, n)
	for i := 0; i < n; i++ {
		msgs = append(msgs, Message{
			TS:          fmt.Sprintf("%d.%06d", 1700000000+i, i),
			ChannelID:   "C1",
			WorkspaceID: "T1",
			UserID:      "U1",
			Text:        "benchmark message body",
			RawJSON:     `{"type":"message","text":"benchmark message body"}`,
			CreatedAt:   int64(1700000000 + i),
		})
	}
	return msgs
}

func newBenchDB(b *testing.B) *DB {
	b.Helper()
	db, err := New(filepath.Join(b.TempDir(), "bench.db"))
	if err != nil {
		b.Fatalf("New: %v", err)
	}
	b.Cleanup(func() { db.Close() })
	return db
}

func BenchmarkUpsertMessage500Individual(b *testing.B) {
	db := newBenchDB(b)
	msgs := benchMessages(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, m := range msgs {
			if err := db.UpsertMessage(m); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkUpsertMessages500Batched(b *testing.B) {
	db := newBenchDB(b)
	msgs := benchMessages(500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := db.UpsertMessages(msgs); err != nil {
			b.Fatal(err)
		}
	}
}
