package slackurl

import "testing"

// mrkdwn keeps the wire-escaped "&amp;"; thread_ts must survive it in
// any parameter position.
func TestParse_WireEscapedAmpersand(t *testing.T) {
	for _, raw := range []string{
		"https://example.slack.com/archives/C999/p1700000050000400?thread_ts=1700000000.000100&amp;cid=C999",
		"https://example.slack.com/archives/C999/p1700000050000400?cid=C999&amp;thread_ts=1700000000.000100",
	} {
		pl, ok := Parse(raw)
		if !ok {
			t.Fatalf("Parse(%q) not ok", raw)
		}
		if string(pl.ThreadTS) != "1700000000.000100" {
			t.Errorf("Parse(%q).ThreadTS = %q", raw, pl.ThreadTS)
		}
	}
}
