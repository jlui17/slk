//go:build unix

package notify

import (
	"os"
	"path/filepath"
	"testing"
)

// Two Leaders on one lock file stand in for two slk instances: the
// advisory lock is per open file, so they contend even in one process.
func TestLeader_SecondInstanceDoesNotLead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify.lock")
	first, second := NewLeader(path), NewLeader(path)

	if !first.IsLeader() {
		t.Fatal("first instance did not take an uncontended lock")
	}
	if second.IsLeader() {
		t.Fatal("second instance led while the first held the lock")
	}
	// Leadership is sticky: the holder keeps emitting without re-taking
	// anything.
	if !first.IsLeader() {
		t.Fatal("first instance stopped leading")
	}
}

func TestLeader_TakesOverWhenTheHolderIsGone(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notify.lock")
	first, second := NewLeader(path), NewLeader(path)

	if !first.IsLeader() || second.IsLeader() {
		t.Fatal("setup: expected the first instance to lead alone")
	}

	// Dropping the lock is what the OS does for a process that exits;
	// nothing in slk releases it deliberately.
	if err := first.lock.Unlock(); err != nil {
		t.Fatalf("release: %v", err)
	}

	if !second.IsLeader() {
		t.Fatal("second instance did not take over after the holder left")
	}
}

func TestLeader_NilAlwaysLeads(t *testing.T) {
	var l *Leader
	if !l.IsLeader() {
		t.Fatal("a nil Leader must not suppress notifications")
	}
}

// The whole point of the election: a losing instance stays quiet for a
// message every instance received.
func TestNotifier_SkipsWhenAnotherInstanceLeads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notify.lock")
	out := filepath.Join(dir, "fired")

	holder := NewLeader(path)
	if !holder.IsLeader() {
		t.Fatal("setup: holder did not take the lock")
	}

	n := New(true, "touch "+out)
	n.SetLeader(NewLeader(path))
	if err := n.Notify("Acme: #general", "someone: hi"); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a non-leading instance ran its notify_command")
	}

	// The leader still notifies.
	if err := New(true, "touch "+out).Notify("Acme: #general", "someone: hi"); err != nil {
		t.Fatalf("Notify (no leader set): %v", err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("an unelected Notifier should still notify: %v", err)
	}
}

func TestStatusReporter_SkipsWhenAnotherInstanceLeads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notify.lock")
	out := filepath.Join(dir, "fired")

	holder := NewLeader(path)
	if !holder.IsLeader() {
		t.Fatal("setup: holder did not take the lock")
	}

	sr := NewStatusReporter("touch " + out)
	sr.SetLeader(NewLeader(path))
	if err := sr.Report(3, 1, "Acme", "slk (3)"); err != nil {
		t.Fatalf("Report: %v", err)
	}
	if _, err := os.Stat(out); err == nil {
		t.Fatal("a non-leading instance ran its status_command")
	}
}
