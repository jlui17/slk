package messages

import "testing"

func TestApplyReaction(t *testing.T) {
	m := MessageItem{}
	ApplyReaction(&m, "tada", "U1", "U1", false)
	ApplyReaction(&m, "tada", "U1", "U1", false) // duplicate echo: no double count
	if len(m.Reactions) != 1 || m.Reactions[0].Count != 1 || !m.Reactions[0].HasReacted {
		t.Fatalf("add: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U2", "U1", false)
	if m.Reactions[0].Count != 2 {
		t.Fatalf("second user: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U9", "U1", true) // not a reactor, list not truncated: ignored
	if m.Reactions[0].Count != 2 {
		t.Fatalf("spurious remove changed the count: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U1", "U1", true)
	if m.Reactions[0].Count != 1 || m.Reactions[0].HasReacted {
		t.Fatalf("remove own: %+v", m.Reactions)
	}
	ApplyReaction(&m, "tada", "U2", "U1", true)
	if len(m.Reactions) != 0 {
		t.Fatalf("last remove did not drop the pill: %+v", m.Reactions)
	}
	m.Reactions = []ReactionItem{{Emoji: "eyes", Count: 5, UserIDs: []string{"U1"}}}
	ApplyReaction(&m, "eyes", "U7", "U1", true) // truncated list: the removal is real
	if m.Reactions[0].Count != 4 {
		t.Fatalf("truncated remove: %+v", m.Reactions)
	}
}

func TestReactionNav(t *testing.T) {
	var n ReactionNav
	n.Left(2) // two pills + "+": wraps to the "+"
	if n.Index != 2 {
		t.Fatalf("Left wrap: %d", n.Index)
	}
	n.Right(2)
	if n.Index != 0 {
		t.Fatalf("Right wrap: %d", n.Index)
	}
	rs := []ReactionItem{{Emoji: "a"}, {Emoji: "b"}}
	if e, plus := n.Selected(rs); e != "a" || plus {
		t.Fatalf("Selected = %q, %v", e, plus)
	}
	n.Index = 5
	n.Clamp(2)
	if n.Index != 2 {
		t.Fatalf("Clamp: %d", n.Index)
	}
	if _, plus := n.Selected(rs); !plus {
		t.Fatal("index past the pills must be the + pill")
	}
}
