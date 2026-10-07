package slackurl

import "testing"

func TestParseProfile(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		sub, user string
		ok        bool
	}{
		{ProfileURL("acme", "U123ABC"), "acme", "U123ABC", true},
		{"https://acme.slack.com/team/W0ENT/", "acme", "W0ENT", true},
		{"http://acme.slack.com/team/U1", "", "", false},
		{"https://acme.example.com/team/U1", "", "", false},
		{"https://acme.slack.com/archives/C1/p1700000000000100", "", "", false},
		{"https://acme.slack.com/team/C1", "", "", false},
		{"https://.slack.com/team/U1", "", "", false},
	} {
		sub, user, ok := ParseProfile(tc.raw)
		if sub != tc.sub || user != tc.user || ok != tc.ok {
			t.Errorf("ParseProfile(%q) = %q, %q, %v; want %q, %q, %v", tc.raw, sub, user, ok, tc.sub, tc.user, tc.ok)
		}
	}
}
