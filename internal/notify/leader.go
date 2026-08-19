package notify

import (
	"sync"

	"github.com/gammons/slk/internal/filelock"
)

// Leader picks which slk instance speaks for the user when several
// share a data directory. Every instance connects to the same
// workspaces and sees the same messages, so without it each one fires
// its own desktop notification and its own status_command run for a
// single incoming message.
//
// The first instance to emit takes an advisory lock on a file in the
// data directory and holds it for the life of the process. Handover
// needs no cooperation: the OS drops the lock when the holder exits, so
// whichever instance emits next picks it up.
type Leader struct {
	mu   sync.Mutex
	lock *filelock.Lock
	held bool
}

// NewLeader returns a Leader contending for the lock file at path.
func NewLeader(path string) *Leader {
	return &Leader{lock: filelock.New(path)}
}

// IsLeader reports whether this process may emit, taking the lock if
// nobody holds it. A nil Leader always leads, so an unwired caller
// behaves exactly as it did before leader election existed.
func (l *Leader) IsLeader() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.held {
		return true
	}
	held, err := l.lock.TryLock()
	if err != nil {
		// An unwritable data directory or a filesystem without locking
		// must not leave the user with no notifications at all;
		// duplicates are the lesser failure.
		return true
	}
	l.held = held
	return held
}
