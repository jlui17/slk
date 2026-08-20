package mrkdwn

import "testing"

func TestMentionedUserIDs(t *testing.T) {
	got := MentionedUserIDs("hey <@U1> and <@W2AB|claude tag>, not <#C3> or <@lower>")
	want := []string{"U1", "W2AB"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
