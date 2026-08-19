//go:build unix

package main

import (
	"path/filepath"
	"testing"

	"github.com/gammons/slk/internal/filelock"
)

// A second slk instance must wait for a save in progress rather than
// read a config someone else is midway through rewriting.
func TestLockConfig_ExcludesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	unlock, err := lockConfig(path)
	if err != nil {
		t.Fatalf("lockConfig: %v", err)
	}

	// The contender stands in for another process, so it takes the
	// sidecar lock directly: a second lockConfig call would block on
	// this process's configWriteMu before ever reaching the file.
	contender := filelock.New(path + ".lock")
	held, err := contender.TryLock()
	if err != nil {
		t.Fatalf("contender TryLock: %v", err)
	}
	if held {
		t.Fatal("contender took the lock while a save held it")
	}

	unlock()

	held, err = contender.TryLock()
	if err != nil {
		t.Fatalf("contender TryLock after release: %v", err)
	}
	if !held {
		t.Fatal("contender could not take the lock after the save released it")
	}
	contender.Unlock()
}
