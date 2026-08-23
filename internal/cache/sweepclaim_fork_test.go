package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func newClaimDB(t *testing.T, path string) *DB {
	t.Helper()
	db, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestTryClaimThreadSweep_FirstClaimWins(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	got, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0), 30*time.Minute)
	if err != nil {
		t.Fatalf("TryClaimThreadSweep: %v", err)
	}
	if !got {
		t.Fatal("first claim refused; want granted")
	}
}

func TestTryClaimThreadSweep_WithinWindowRefused(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	if _, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0), 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0).Add(29*time.Minute), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("claim inside the window granted; want refused")
	}
}

func TestTryClaimThreadSweep_AfterWindowReclaimable(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	if _, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0), 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0).Add(30*time.Minute), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("claim after the window refused; want granted")
	}
}

func TestTryClaimThreadSweep_WorkspacesAreIndependent(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	if _, err := db.TryClaimThreadSweep("T1", time.Unix(1000, 0), 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	got, err := db.TryClaimThreadSweep("T2", time.Unix(1000, 0), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("T2's first claim refused because T1 holds one; workspaces must claim independently")
	}
}

// Two handles on one file model two slk instances sharing cache.db:
// exactly one of a same-instant claim pair may win.
func TestTryClaimThreadSweep_CrossConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	a := newClaimDB(t, path)
	b := newClaimDB(t, path)

	now := time.Unix(1000, 0)
	gotA, err := a.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := b.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !gotA || gotB {
		t.Fatalf("claims (A=%v, B=%v); want exactly the first connection to win", gotA, gotB)
	}
}
