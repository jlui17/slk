//go:build !unix

package filelock

// Windows has no flock(2), and slk's Windows build does no
// cross-process coordination: every caller proceeds as if it holds the
// lock, which is the behavior every platform had before locking
// existed.

func (l *Lock) TryLock() (bool, error) { return true, nil }

func (l *Lock) Unlock() error { return nil }
