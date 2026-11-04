package messages

import "testing"

// The margin shows hour and minute on the user's clock: a 24-hour
// format stays 24-hour even when it is too long for the margin.
func TestMarginTime(t *testing.T) {
	t.Cleanup(func() { SetTimestampFormat("") })
	ts := tsAt(15, 4, 59)
	for _, c := range []struct{ format, shown, want string }{
		{"3:04 PM", "3:04 PM", "3:04"},
		{"15:04", "15:04", "15:04"},
		{"15:04:05", "15:04:59", "15:04"},    // too long: 24-hour fallback
		{"3:04:05 PM", "3:04:59 PM", "3:04"}, // too long: 12-hour fallback
		{"Jan 2 15:04", "Sep 29 15:04", "15:04"},
	} {
		SetTimestampFormat(c.format)
		if got := MarginTime(MessageItem{TS: ts, Timestamp: c.shown}); got != c.want {
			t.Errorf("format %q, shown %q: MarginTime = %q, want %q", c.format, c.shown, got, c.want)
		}
	}
}
