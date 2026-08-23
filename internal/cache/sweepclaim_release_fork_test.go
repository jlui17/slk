package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestReleaseThreadSweepClaim_ReopensTheWindow(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	now := time.Unix(1000, 0)
	if _, err := db.TryClaimThreadSweep("T1", now, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := db.ReleaseThreadSweepClaim("T1", now); err != nil {
		t.Fatal(err)
	}
	got, err := db.TryClaimThreadSweep("T1", now.Add(time.Second), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if !got {
		t.Fatal("claim refused after release; a failed sweep must not block siblings for the window")
	}
}

// Release is compare-and-clear on the claimant's own stamp: a release
// racing a sibling's newer claim must not erase that claim.
func TestReleaseThreadSweepClaim_IgnoresSomeoneElsesClaim(t *testing.T) {
	db := newClaimDB(t, filepath.Join(t.TempDir(), "c.db"))
	mine := time.Unix(1000, 0)
	if _, err := db.TryClaimThreadSweep("T1", mine, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	theirs := mine.Add(31 * time.Minute)
	if _, err := db.TryClaimThreadSweep("T1", theirs, 30*time.Minute); err != nil {
		t.Fatal(err)
	}
	if err := db.ReleaseThreadSweepClaim("T1", mine); err != nil {
		t.Fatal(err)
	}
	got, err := db.TryClaimThreadSweep("T1", theirs.Add(time.Second), 30*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if got {
		t.Fatal("stale release erased a sibling's newer claim")
	}
}
