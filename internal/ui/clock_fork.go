// internal/ui/clock_fork.go
//
// Testable clock seam for typingTracker, selfSendDedup,
// presenceController, and App, mirroring sidebar.SetNowFunc: each type
// carries a nowFn clock field (declared alongside its struct) that
// defaults to time.Now via the nil-safe Now.
package ui

import "time"

type clock func() time.Time

func (c clock) Now() time.Time {
	if c != nil {
		return c()
	}
	return time.Now()
}

func (t *typingTracker) now() time.Time { return t.nowFn.Now() }

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (t *typingTracker) SetNowFunc(fn func() time.Time) { t.nowFn = fn }

func (d *selfSendDedup) now() time.Time { return d.nowFn.Now() }

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (d *selfSendDedup) SetNowFunc(fn func() time.Time) { d.nowFn = fn }

func (p *presenceController) now() time.Time { return p.nowFn.Now() }

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (p *presenceController) SetNowFunc(fn func() time.Time) { p.nowFn = fn }

func (a *App) now() time.Time { return a.nowFn.Now() }

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (a *App) SetNowFunc(fn func() time.Time) { a.nowFn = fn }
