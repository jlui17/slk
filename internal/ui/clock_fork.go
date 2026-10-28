// internal/ui/clock_fork.go
//
// Testable clock seam for typingTracker, selfSendDedup,
// presenceController, and App, mirroring sidebar.SetNowFunc:
// presenceController carries a nowFn clock field (declared alongside its
// struct) that defaults to time.Now via the nil-safe Now; the others set
// upstream's now field.
package ui

import "time"

type clock func() time.Time

func (c clock) Now() time.Time {
	if c != nil {
		return c()
	}
	return time.Now()
}

// SetNowFunc injects a clock for tests into the tracker's now field
// (upstream's). Pass nil to revert to time.Now.
func (t *typingTracker) SetNowFunc(fn func() time.Time) {
	if fn == nil {
		fn = time.Now
	}
	t.now = fn
}

// SetNowFunc injects a clock for tests into the dedup's now field
// (upstream's). Pass nil to revert to time.Now.
func (d *selfSendDedup) SetNowFunc(fn func() time.Time) {
	if fn == nil {
		fn = time.Now
	}
	d.now = fn
}

func (p *presenceController) now() time.Time { return p.nowFn.Now() }

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func (p *presenceController) SetNowFunc(fn func() time.Time) { p.nowFn = fn }

// SetNowFunc injects a clock for tests into App's now field (upstream's,
// which the user-profile dialog reads). Pass nil to revert to time.Now.
func (a *App) SetNowFunc(fn func() time.Time) {
	if fn == nil {
		fn = time.Now
	}
	a.now = fn
}
