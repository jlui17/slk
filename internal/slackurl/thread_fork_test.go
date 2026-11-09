package slackurl

import "testing"

func TestThreadPermalinkParsesBack(t *testing.T) {
	link := ThreadPermalink("acme", "C123ABC", "1700000000.000100")
	want := "https://acme.slack.com/archives/C123ABC/p1700000000000100?thread_ts=1700000000.000100&cid=C123ABC"
	if link != want {
		t.Fatalf("ThreadPermalink = %q, want %q", link, want)
	}
	pl, ok := Parse(link)
	if !ok {
		t.Fatalf("Parse(%q) not ok", link)
	}
	if pl.Subdomain != "acme" || pl.ChannelID != "C123ABC" || pl.MessageTS != "1700000000.000100" || pl.ThreadTS != "1700000000.000100" {
		t.Errorf("Parse(%q) = %+v", link, pl)
	}
}

func TestThreadPermalinkEmptyWithoutParts(t *testing.T) {
	for _, c := range [][3]string{{"", "C1", "1.000001"}, {"acme", "", "1.000001"}, {"acme", "C1", ""}} {
		if got := ThreadPermalink(c[0], c[1], c[2]); got != "" {
			t.Errorf("ThreadPermalink(%q, %q, %q) = %q, want empty", c[0], c[1], c[2], got)
		}
	}
}
