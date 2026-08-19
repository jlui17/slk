//go:build unix

package filelock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// Lock blocks until this process holds the lock.
func (l *Lock) Lock() error {
	return l.acquire(unix.LOCK_EX)
}

// TryLock reports whether the lock was acquired without waiting. A
// false result with a nil error means another process holds it.
func (l *Lock) TryLock() (bool, error) {
	err := l.acquire(unix.LOCK_EX | unix.LOCK_NB)
	if errors.Is(err, unix.EWOULDBLOCK) {
		return false, nil
	}
	return err == nil, err
}

// Unlock releases the lock. Calling it without holding the lock is a
// no-op.
func (l *Lock) Unlock() error {
	if l.f == nil {
		return nil
	}
	f := l.f
	l.f = nil
	// Closing the descriptor drops the flock; there is no window where
	// the lock outlives the handle.
	return f.Close()
}

// acquire opens the lock file and flocks it with the given how flags.
//
// flock(2) locks belong to the open file description, not the process,
// so two Locks on one path contend even inside a single process.
func (l *Lock) acquire(how int) error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	if err := unix.Flock(int(f.Fd()), how); err != nil {
		f.Close()
		return err
	}
	l.f = f
	return nil
}
