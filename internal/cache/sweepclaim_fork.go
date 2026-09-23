package cache

import (
	"path/filepath"
	"time"

	"github.com/gammons/slk/internal/filelock"
)

// ThreadSweepClaim is a claim this process holds on a workspace's
// thread-subscription sweep. The process holds the workspace's lock
// file from TryClaimThreadSweep until Close, so the claim cannot outlive
// the process: the OS drops the lock at exit, however the process
// exits, and the next sibling takes over.
type ThreadSweepClaim struct {
	db          *DB
	workspaceID string
	claimedAt   time.Time
	lock        *filelock.Lock
}

// TryClaimThreadSweep claims the right to run a workspace's
// thread-subscription sweep, or returns nil, nil when a sibling holds
// it. Concurrent slk instances share cache.db and the sweep's writes
// land there, so one sweep per window serves the fleet.
//
// Two things stand in the way of a claim. The workspace's lock file,
// held for the life of the returned claim, marks a sweep in flight. The
// thread_sweep_claims row, once Complete stamps it, marks a finished
// sweep whose results stand for the window. A row that is uncompleted
// while we hold the lock belongs to a holder that is gone — a live one
// would still hold the lock — so it is taken over.
func (db *DB) TryClaimThreadSweep(workspaceID string, now time.Time, window time.Duration) (*ThreadSweepClaim, error) {
	lock := filelock.New(db.threadSweepLockPath(workspaceID))
	ok, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	res, err := db.conn.Exec(`
		INSERT INTO thread_sweep_claims (workspace_id, claimed_at, completed_at) VALUES (?, ?, 0)
		ON CONFLICT(workspace_id) DO UPDATE SET claimed_at = excluded.claimed_at, completed_at = 0
		WHERE thread_sweep_claims.completed_at = 0
		   OR excluded.claimed_at - thread_sweep_claims.claimed_at >= ?`,
		workspaceID, now.Unix(), int64(window.Seconds()))
	if err != nil {
		lock.Unlock()
		return nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		lock.Unlock()
		return nil, err
	}
	if n != 1 {
		lock.Unlock()
		return nil, nil
	}
	return &ThreadSweepClaim{db: db, workspaceID: workspaceID, claimedAt: now, lock: lock}, nil
}

// Complete records that the sweep finished, so the claim paces siblings
// for the window. Compare-and-stamp on the claimant's own claimed_at.
func (c *ThreadSweepClaim) Complete(now time.Time) error {
	_, err := c.db.conn.Exec(`
		UPDATE thread_sweep_claims SET completed_at = ?
		WHERE workspace_id = ? AND claimed_at = ?`,
		now.Unix(), c.workspaceID, c.claimedAt.Unix())
	return err
}

// Close drops the lock. Safe to call more than once.
func (c *ThreadSweepClaim) Close() error {
	return c.lock.Unlock()
}

func (db *DB) threadSweepLockPath(workspaceID string) string {
	return filepath.Join(db.dir, "thread-sweep-"+workspaceID+".lock")
}
