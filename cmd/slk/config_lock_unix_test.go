//go:build unix

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/filelock"
)

// A second slk instance must wait for a save in progress rather than
// read a config someone else is midway through rewriting.
func TestLockConfig_ExcludesASecondHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	unlock, _, err := lockConfig(path)
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

// A wedged holder must not freeze the UI. The theme and width savers
// run on bubbletea's Update goroutine, so a save waits only briefly for
// another instance before going ahead unlocked.
func TestLockConfig_GivesUpOnAWedgedHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	wedged := filelock.New(path + ".lock")
	held, err := wedged.TryLock()
	if err != nil || !held {
		t.Fatalf("setup: holder TryLock = (%v, %v)", held, err)
	}
	defer wedged.Unlock()

	start := time.Now()
	if err := saveGlobalTheme(path, "nord"); err != nil {
		t.Fatalf("saveGlobalTheme: %v", err)
	}
	waited := time.Since(start)

	if waited < configLockWait {
		t.Errorf("save returned after %s, before the %s bound: it never waited for the holder", waited, configLockWait)
	}
	// Generous slack over the bound so a loaded machine doesn't fail a
	// save that did give up on time.
	if limit := 20 * configLockWait; waited > limit {
		t.Errorf("save waited %s on a wedged holder; want it to give up within %s", waited, limit)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Appearance.Theme != "nord" {
		t.Errorf("theme = %q; want nord: the save gave up on the lock but must still write", cfg.Appearance.Theme)
	}
}

// Onboarding is the one config writer that must not fall back to an
// unlocked write. Two unlocked appends pick the same slug and leave a
// config go-toml refuses to parse, which stops slk from starting at
// all — worse than asking the user to retry.
func TestAppendWorkspaceConfigBlock_RefusesWhenAnotherInstanceHoldsTheLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	holder := filelock.New(path + ".lock")
	held, err := holder.TryLock()
	if err != nil || !held {
		t.Fatalf("setup: holder TryLock = (%v, %v)", held, err)
	}
	defer holder.Unlock()

	err = appendWorkspaceConfigBlock(path, "acme", "T01ABCDEF", "ACME")
	if err == nil {
		t.Fatal("append wrote the block while another instance held the lock")
	}
	if !strings.Contains(err.Error(), path+".lock") {
		t.Errorf("error %q does not name the lock file the user has to deal with", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("config.toml exists after the refusal (stat: %v); the append must write nothing", err)
	}
}
