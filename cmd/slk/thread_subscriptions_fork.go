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
	claimed, err := s.db.TryClaimThreadSweep(s.workspaceID, time.Now(), window)
	if err == nil && !claimed {
		debuglog.Backfill("team=%s subscription-sync skipped: sibling instance holds the sweep claim", s.workspaceID)
		return nil
	}
	return s.sync(ctx)
}
