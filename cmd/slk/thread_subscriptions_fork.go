package main

import (
	"context"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

// syncIfUnclaimed runs sync only when this instance wins the shared
// per-workspace sweep claim (cache.DB.TryClaimThreadSweep): concurrent
// instances share cache.db and the sweep's writes land there, so a
// sibling's sweep inside the window substitutes for ours. A claim
// error fails open — a broken claim table must not cost thread
// catch-up. The skip returns nil so the caller's onDone still fires
// and the Threads view re-reads what the sibling wrote.
func (s *threadSubscriptionSync) syncIfUnclaimed(ctx context.Context, window time.Duration) error {
	now := time.Now()
	claimed, claimErr := s.db.TryClaimThreadSweep(s.workspaceID, now, window)
	if claimErr == nil && !claimed {
		debuglog.Backfill("team=%s subscription-sync skipped: sibling instance holds the sweep claim", s.workspaceID)
		return nil
	}
	err := s.sync(ctx)
	if err != nil && claimErr == nil {
		// A transient getView failure must not block every sibling for
		// the window; reconnect/wake is exactly when failures cluster.
		if relErr := s.db.ReleaseThreadSweepClaim(s.workspaceID, now); relErr != nil {
			debuglog.Backfill("team=%s subscription-sync claim release err=%v", s.workspaceID, relErr)
		}
	}
	return err
}
