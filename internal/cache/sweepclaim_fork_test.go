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

// mustClaim claims T1's sweep at now and fails the test if refused. The
// claim is closed at cleanup; a test that needs the lock dropped
// earlier closes it itself.
func mustClaim(t *testing.T, db *DB, now time.Time) *ThreadSweepClaim {
	t.Helper()
	claim, err := db.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil {
		t.Fatalf("TryClaimThreadSweep: %v", err)
	}
	if claim == nil {
		t.Fatal("claim refused; want granted")
	}
	t.Cleanup(func() { claim.Close() })
	return claim
}

// completeAndClose finishes a sweep the way syncIfUnclaimed does after
// a successful sync.
func completeAndClose(t *testing.T, claim *ThreadSweepClaim, now time.Time) {
	t.Helper()
	if err := claim.Complete(now); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if err := claim.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestTryClaimThreadSweep_FirstClaimWins(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	mustClaim(t, db, time.Unix(1000, 0))
}

func TestTryClaimThreadSweep_WithinWindowRefused(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	completeAndClose(t, mustClaim(t, db, now), now)
	got, err := db.TryClaimThreadSweep("T1", now.Add(29*time.Minute), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		got.Close()
		t.Fatal("claim inside the window granted; want refused")
	}
}

func TestTryClaimThreadSweep_AfterWindowReclaimable(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	completeAndClose(t, mustClaim(t, db, now), now)
	mustClaim(t, db, now.Add(30*time.Minute))
}

func TestTryClaimThreadSweep_WorkspacesAreIndependent(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	mustClaim(t, db, now)
	got, err := db.TryClaimThreadSweep("T2", now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("T2's first claim refused because T1 holds one; workspaces must claim independently")
	}
	got.Close()
}

// Two handles on one file model two slk instances sharing cache.db:
// exactly one of a same-instant claim pair may win.
func TestTryClaimThreadSweep_CrossConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.db")
	a := newClaimDB(t, path)
	b := newClaimDB(t, path)

	now := time.Unix(1000, 0)
	mustClaim(t, a, now)
	gotB, err := b.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if gotB != nil {
		gotB.Close()
		t.Fatal("both connections won a same-instant claim; want exactly the first")
	}
}

// While A sweeps, B is refused by the lock; once A completes, B is
// refused by the window.
func TestTryClaimThreadSweep_LiveHolderRefusesSibling(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	a := mustClaim(t, db, now)

	b, err := db.TryClaimThreadSweep("T1", now, 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		b.Close()
		t.Fatal("sibling granted the claim while the holder is mid-sweep; want refused")
	}

	completeAndClose(t, a, now.Add(time.Minute))
	b, err = db.TryClaimThreadSweep("T1", now.Add(2*time.Minute), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		b.Close()
		t.Fatal("sibling granted the claim inside a completed sweep's window; want refused")
	}
}

// Close without Complete is what a holder that failed or died leaves
// behind: an uncompleted row and no lock. The next sibling takes it
// over instead of waiting out the window.
func TestTryClaimThreadSweep_AbandonedClaimIsTakenOver(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	a := mustClaim(t, db, now)
	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mustClaim(t, db, now.Add(time.Second))
}

// A completed sweep's claim is what spares the siblings their own sweep
// for the window, with or without the holder still running.
func TestTryClaimThreadSweep_CompletedClaimPacesTheWindow(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	completeAndClose(t, mustClaim(t, db, now), now)

	b, err := db.TryClaimThreadSweep("T1", now.Add(29*time.Minute), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if b != nil {
		b.Close()
		t.Fatal("claim granted 29m after a completed sweep; want refused until the window ends")
	}
	mustClaim(t, db, now.Add(30*time.Minute))
}
