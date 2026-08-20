package main

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/filelock"
)

// configLockPath is the sidecar whose file lock guards configPath.
func configLockPath(configPath string) string {
	return configPath + ".lock"
}

// lockConfig takes both locks guarding a read-modify-write cycle on
// configPath — configWriteMu for this process, an advisory file lock
// for every other slk instance — and returns the release for both plus
// whether the file lock was actually taken.
//
// The whole cycle has to run under it, not just the write: each saver
// rewrites the entire file from the copy it read, so two instances
// interleaving read and write lose one another's update even though
// writeConfigAtomic makes each write indivisible.
//
// A false held is the caller's decision to make. Overwriting one
// setting is worth the risk of a lost update; appending a workspace
// block is not, because two unlocked appends produce a config go-toml
// refuses to parse at all.
//
// The lock lives on a sidecar file rather than on config.toml, because
// writeConfigAtomic renames a fresh file over the config: a lock taken
// on config.toml would be left holding an inode no later writer opens.
func lockConfig(configPath string) (unlock func(), held bool, err error) {
	configWriteMu.Lock()
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		configWriteMu.Unlock()
		return nil, false, err
	}
	lock := filelock.New(configLockPath(configPath))
	held = acquireConfigLock(lock)
	return func() {
		if held {
			lock.Unlock()
		}
		configWriteMu.Unlock()
	}, held, nil
}

// configLockWait bounds how long a saver waits for another instance to
// finish. The locked section is a single small read, rewrite and
// rename, so a wait this long already means the holder is wedged (a
// stopped process, a hung filesystem) rather than busy. Waiting longer
// would freeze the TUI, since the theme and width savers run on
// bubbletea's Update goroutine. A locked section that ever grows beyond
// one file rewrite makes this value wrong.
//
// It bounds one acquisition, not a saver's total wait: configWriteMu
// queues this process's savers behind each other, so a theme save
// landing while W workspaces boot behind a wedged holder waits for
// their version_ts refreshes to time out first, roughly (W+1) times
// this value.
const configLockWait = 250 * time.Millisecond

// acquireConfigLock takes lock, reporting whether it got it. It gives
// up after configLockWait, and on any error.
//
// Neither outcome is a reason to refuse the save. The errors are
// environmental — a filesystem with no flock, a config directory that
// turned read-only — and none of them stop the atomic write that
// follows. Saving unlocked risks a lost update only if a second
// instance saves in the same instant; refusing means the user's theme
// silently stops persisting, every time.
func acquireConfigLock(lock *filelock.Lock) bool {
	deadline := time.Now().Add(configLockWait)
	for {
		held, err := lock.TryLock()
		if err != nil {
			debuglog.General("config lock unavailable, saving unlocked: %v", err)
			return false
		}
		if held {
			return true
		}
		if time.Now().After(deadline) {
			debuglog.General("config lock held by another instance for %s, saving unlocked", configLockWait)
			return false
		}
		time.Sleep(10 * time.Millisecond)
	}
}
