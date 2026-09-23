package main

import (
	"context"
	"time"

	"github.com/gammons/slk/internal/debuglog"
)

// syncIfUnclaimed runs sync only when this instance wins the shared
// per-workspace sweep claim (cache.DB.TryClaimThreadSweep): concurrent
// instances share cache.db and the sweep's writes land there, so a
// sibling's sweep inside the window substitutes for ours. The claim
// holds a lock file for the life of the sweep and is completed only
// after sync succeeds, so a sweep that fails, is cancelled, or dies
// with the process leaves an uncompleted row the next sibling takes
// over. A claim error fails open — a broken claim table must not cost
// thread catch-up. The skip returns nil so the caller's onDone still
// fires and the Threads view re-reads what the sibling wrote.
func (s *threadSubscriptionSync) syncIfUnclaimed(ctx context.Context, window time.Duration) error {
	claim, claimErr := s.db.TryClaimThreadSweep(s.workspaceID, time.Now(), window)
	if claimErr == nil && claim == nil {
		debuglog.Backfill("team=%s subscription-sync skipped: sibling instance holds the sweep claim", s.workspaceID)
		return nil
	}
	if claimErr == nil {
		defer claim.Close()
	}
	err := s.sync(ctx)
	if err == nil && claimErr == nil {
		if cErr := claim.Complete(time.Now()); cErr != nil {
			debuglog.Backfill("team=%s subscription-sync claim complete err=%v", s.workspaceID, cErr)
		}
	}
	return err
}
