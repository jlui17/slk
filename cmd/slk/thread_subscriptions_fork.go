package main

import (
	"context"
	"sync"
	"time"

	"github.com/gammons/slk/internal/cache"
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
	if claimErr == nil {
		inFlightThreadSweepClaims.Store(s.workspaceID, now)
		defer inFlightThreadSweepClaims.Delete(s.workspaceID)
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

// inFlightThreadSweepClaims is the claims this process holds for
// sweeps that have not returned: workspace ID -> claimedAt. The
// per-workspace gate admits one sweep at a time, so the workspace ID
// identifies the claim.
var inFlightThreadSweepClaims sync.Map

// releaseInFlightThreadSweepClaims gives back the claims of sweeps the
// exiting process is about to abandon. Quit cancels nothing — both
// trigger sites hand the sweep context.Background() — so a sweep in
// flight when the UI loop exits dies with the process, having written
// nothing, and its claim would cost every sibling the sweep for the
// whole window. Call after the UI loop has exited, while db is open.
func releaseInFlightThreadSweepClaims(db *cache.DB) {
	inFlightThreadSweepClaims.Range(func(workspaceID, claimedAt any) bool {
		if err := db.ReleaseThreadSweepClaim(workspaceID.(string), claimedAt.(time.Time)); err != nil {
			debuglog.Backfill("team=%s subscription-sync claim release at quit err=%v", workspaceID, err)
		}
		return true
	})
}
