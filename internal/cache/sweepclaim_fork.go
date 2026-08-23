package cache

import "time"

// TryClaimThreadSweep atomically claims the right to run a workspace's
// thread-subscription sweep. The claim lives in the shared cache.db so
// concurrent slk instances elect one sweeper per window: the sweep
// writes everything it learns into this same DB, so a sibling's recent
// sweep substitutes for running another. Returns true when this caller
// won the claim (no claim yet, or the standing one is at least window
// old).
func (db *DB) TryClaimThreadSweep(workspaceID string, now time.Time, window time.Duration) (bool, error) {
	res, err := db.conn.Exec(`
		INSERT INTO thread_sweep_claims (workspace_id, claimed_at) VALUES (?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET claimed_at = excluded.claimed_at
		WHERE excluded.claimed_at - thread_sweep_claims.claimed_at >= ?`,
		workspaceID, now.Unix(), int64(window.Seconds()))
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// ReleaseThreadSweepClaim clears a claim this caller took at claimedAt
// so a failed sweep doesn't block every sibling for the window.
// Compare-and-clear on the claimant's own stamp: a sibling's newer
// claim is left standing.
func (db *DB) ReleaseThreadSweepClaim(workspaceID string, claimedAt time.Time) error {
	_, err := db.conn.Exec(`
		DELETE FROM thread_sweep_claims
		WHERE workspace_id = ? AND claimed_at = ?`,
		workspaceID, claimedAt.Unix())
	return err
}
