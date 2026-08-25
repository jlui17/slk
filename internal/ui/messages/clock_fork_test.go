package messages

import (
	"testing"
	"time"
)

func TestFormatDateSeparatorUsesInjectedClock(t *testing.T) {
	// Wednesday.
	SetNowFunc(func() time.Time {
		return time.Date(2026, 8, 19, 15, 30, 0, 0, time.UTC)
	})
	defer SetNowFunc(nil)

	cases := []struct{ date, want string }{
		{"2026-08-19", "Today"},
		{"2026-08-18", "Yesterday"},
		{"2026-08-15", "Saturday"},
		{"2026-08-01", "Saturday, August 1, 2026"},
	}
	for _, c := range cases {
		if got := FormatDateSeparator(c.date); got != c.want {
			t.Errorf("FormatDateSeparator(%q): want %q, got %q", c.date, c.want, got)
		}
	}
}
