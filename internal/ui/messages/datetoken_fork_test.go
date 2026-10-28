package messages

import (
	"strconv"
	"testing"
	"time"
)

// The real reply: "todos as of <!date^1793169412^{time} ({ago})|06:36 UTC>"
// drew "11:36 PM ({ago})" with {ago} left raw.
func TestFormatSlackDateTokenIn_Ago(t *testing.T) {
	ts := strconv.FormatInt(time.Now().Add(-5*time.Minute-10*time.Second).Unix(), 10)
	if got := formatSlackDateTokenIn(ts, "{ago}", "FB", "", utcZone()); got != "5 minutes ago" {
		t.Errorf("{ago} = %q, want %q", got, "5 minutes ago")
	}
	want := time.Unix(mustParseInt(t, ts), 0).In(utcZone()).Format("3:04 PM") + " (5 minutes ago)"
	if got := formatSlackDateTokenIn(ts, "{time} ({ago})", "06:36 UTC", "", utcZone()); got != want {
		t.Errorf("{time} ({ago}) = %q, want %q", got, want)
	}
}

func TestSlackDateAgo(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		d    time.Duration
		want string
	}{
		{20 * time.Second, "just now"},
		{-20 * time.Second, "just now"},
		{time.Minute, "1 minute ago"},
		{3*time.Minute + 59*time.Second, "3 minutes ago"},
		{time.Hour, "1 hour ago"},
		{4*time.Hour + 30*time.Minute, "4 hours ago"},
		{24 * time.Hour, "1 day ago"},
		{2 * 24 * time.Hour, "2 days ago"},
		{45 * 24 * time.Hour, "1 month ago"},
		{200 * 24 * time.Hour, "6 months ago"},
		{400 * 24 * time.Hour, "1 year ago"},
		{3 * 365 * 24 * time.Hour, "3 years ago"},
		{-3 * time.Minute, "in 3 minutes"},
		{-2 * 24 * time.Hour, "in 2 days"},
	}
	for _, tc := range cases {
		if got := slackDateAgo(now.Add(-tc.d), now); got != tc.want {
			t.Errorf("slackDateAgo(now-%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}

// A format with a token slk does not know draws the fallback, not the
// token's raw braces.
func TestFormatSlackDateTokenIn_UnknownTokenFallsBack(t *testing.T) {
	if got := formatSlackDateTokenIn(testTS, "{time} ({someday})", "06:36 UTC", "", utcZone()); got != "06:36 UTC" {
		t.Errorf("unknown token = %q, want fallback %q", got, "06:36 UTC")
	}
}

func mustParseInt(t *testing.T, s string) int64 {
	t.Helper()
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
