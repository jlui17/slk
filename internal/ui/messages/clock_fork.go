package messages

import "time"

// nowFn is the clock behind FormatDateSeparator's "Today"/"Yesterday"
// day-boundary math. Package-level because FormatDateSeparator is a
// package-level func shared by the channel pane and thread panel.
var nowFn = time.Now

// SetNowFunc injects a clock for tests. Pass nil to revert to time.Now.
func SetNowFunc(fn func() time.Time) {
	if fn == nil {
		fn = time.Now
	}
	nowFn = fn
}
