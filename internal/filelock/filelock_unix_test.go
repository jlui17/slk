//go:build unix

package filelock

import (
	"path/filepath"
	"testing"
)

// Two Locks on one path stand in for two slk processes: flock(2) locks
// belong to the open file description, so separate Locks contend even
// though this test is a single process.
func TestTryLock_SecondHolderIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.lock")
	first, second := New(path), New(path)

	ok, err := first.TryLock()
	if err != nil {
		t.Fatalf("first TryLock: %v", err)
	}
	if !ok {
		t.Fatal("first TryLock did not acquire an uncontended lock")
	}

	ok, err = second.TryLock()
	if err != nil {
		t.Fatalf("second TryLock: %v", err)
	}
	if ok {
		t.Fatal("second TryLock acquired a lock the first holder still holds")
	}

	if err := first.Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
	ok, err = second.TryLock()
	if err != nil {
		t.Fatalf("second TryLock after release: %v", err)
	}
	if !ok {
		t.Fatal("second TryLock did not acquire the released lock")
	}
}

// Unlock on a Lock that never acquired anything is a no-op, so a saver
// can defer it before knowing whether the lock succeeded.
func TestUnlock_WithoutLockIsNoop(t *testing.T) {
	if err := New(filepath.Join(t.TempDir(), "x.lock")).Unlock(); err != nil {
		t.Fatalf("Unlock: %v", err)
	}
}
