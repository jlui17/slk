package slackurl

import (
	"testing"

	"github.com/gammons/slk/internal/ids"
)

func TestParseChannel(t *testing.T) {
	for _, tc := range []struct {
		raw     string
		sub     string
		channel ids.ChannelID
		ok      bool
	}{
		{"https://acme.slack.com/archives/C04PAYENG", "acme", "C04PAYENG", true},
		{"https://acme.slack.com/archives/C04PAYENG/", "acme", "C04PAYENG", true},
		{"https://acme.slack.com/archives/C1/p1700000000000100", "", "", false},
		{"http://acme.slack.com/archives/C1", "", "", false},
		{"https://acme.example.com/archives/C1", "", "", false},
		{"https://acme.slack.com/docs/T1/F1", "", "", false},
	} {
		sub, channel, ok := ParseChannel(tc.raw)
		if sub != tc.sub || channel != tc.channel || ok != tc.ok {
			t.Errorf("ParseChannel(%q) = %q, %q, %v; want %q, %q, %v", tc.raw, sub, channel, ok, tc.sub, tc.channel, tc.ok)
		}
	}
}
