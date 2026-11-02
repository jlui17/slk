package messages

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/peerstatus"
)

// richTextMention returns Block Kit whose only mention of userID is a
// rich_text user element: the ID is in no text field.
func richTextMention(t *testing.T, userID string) []blockkit.Block {
	t.Helper()
	var p struct {
		Blocks slack.Blocks `json:"blocks"`
	}
	raw := `{"blocks":[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[` +
		`{"type":"text","text":"ping "},{"type":"user","user_id":"` + userID + `"}]}]}]}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return blockkit.Parse(p.Blocks)
}

// Every event the channel pane handles without a full rebuild leaves
// the pane exactly as a full rebuild would (assertMatchesFullRebuild),
// rendering only the entries it changed. A dependency an event misses
// shows here as a row that differs from the full rebuild's.
func TestModel_TargetedInvalidationMatchesFullRebuild(t *testing.T) {
	base := prMessages(7) // U1 wrote the even ones, U2 the odd ones
	base[2].Text = "hi <@U7>"
	base[4].Text = "ping"
	base[4].Blocks = richTextMention(t, "U8")
	base[6].Reactions = []ReactionItem{{Emoji: "eyes", Count: 1}, {Emoji: "tada", Count: 2}}
	base[3].Reactions = []ReactionItem{{Emoji: "eyes", Count: 1}}
	ts := tsOf(base)
	odd := []string{ts[1], ts[3], ts[5]}
	even := []string{ts[0], ts[2], ts[4], ts[6]}
	later := time.Now().Add(time.Hour)

	cases := []struct {
		name     string
		msgs     func() []MessageItem // the starting messages; base when nil
		setup    func(m *Model)       // before the first View
		event    func(m *Model)
		rendered []string // entries rendered again by the event's View
	}{
		{name: "an author's name", event: func(m *Model) { m.PatchUserName("U1", "alicia") }, rendered: even},
		{name: "a name mentioned in text", event: func(m *Model) { m.PatchUserName("U7", "gus") }, rendered: []string{ts[2]}},
		{name: "a name mentioned in a block", event: func(m *Model) { m.PatchUserName("U8", "hal") }, rendered: []string{ts[4]}},
		{name: "a name nobody here uses", event: func(m *Model) { m.PatchUserName("U99", "zed") }},
		{
			name:     "an author's status",
			event:    func(m *Model) { m.PatchUserStatus("U2", peerstatus.Status{Emoji: ":calendar:"}) },
			rendered: odd,
		},
		{
			name: "an author's status expires",
			setup: func(m *Model) {
				m.SetUserStatuses(map[string]peerstatus.Status{"U2": {Emoji: ":calendar:", Expires: later}})
			},
			event:    func(m *Model) { m.ExpireStatuses(later.Add(time.Minute)) },
			rendered: odd,
		},
		{name: "another client marks read", event: func(m *Model) { m.SetLastReadTS(ts[3]) }},
		{
			name:  "the new divider moves",
			setup: func(m *Model) { m.SetLastReadTS(ts[1]) },
			event: func(m *Model) { m.SetLastReadTS(ts[5]) },
		},
		{name: "reaction nav starts", event: func(m *Model) { m.EnterReactionNav() }, rendered: []string{ts[6]}},
		{
			name:     "reaction nav right",
			setup:    func(m *Model) { m.EnterReactionNav() },
			event:    func(m *Model) { m.ReactionNavRight() },
			rendered: []string{ts[6]},
		},
		{
			name:     "reaction nav left",
			setup:    func(m *Model) { m.EnterReactionNav() },
			event:    func(m *Model) { m.ReactionNavLeft() },
			rendered: []string{ts[6]},
		},
		{
			name:     "reaction nav ends",
			setup:    func(m *Model) { m.EnterReactionNav() },
			event:    func(m *Model) { m.ExitReactionNav() },
			rendered: []string{ts[6]},
		},
		{
			// The highlight was drawn on the last message; the cursor
			// moved under the nav, and a reaction on the new cursor
			// message clamps the nav: both entries change.
			name:  "reaction nav after the cursor moved",
			setup: func(m *Model) { m.EnterReactionNav() },
			event: func(m *Model) {
				m.SelectByIndex(3)
				_ = m.View(prH, prW)
				m.UpdateReaction(ts[3], "tada", "U9", false)
			},
			rendered: []string{ts[3], ts[6]},
		},
		{name: "the fetch lands unchanged", event: func(m *Model) { m.SetMessages(slices.Clone(base)) }},
		{
			name: "the fetch lands with an edit, a new message and one gone",
			event: func(m *Model) {
				msgs := slices.Clone(base[1:])
				msgs[2].Text = "edited"
				msgs = append(msgs, MessageItem{TS: "1700400000.000100", UserID: "U3", UserName: "carol", Text: "new", Timestamp: "11:00 AM"})
				m.SetMessages(msgs)
			},
			rendered: []string{ts[3], ts[6], "1700400000.000100"},
		},
		{
			name: "another channel",
			event: func(m *Model) {
				m.SetChannel("random", "")
				m.SetMessages([]MessageItem{
					{TS: "1600000000.000100", UserID: "U1", UserName: "alice", Text: "elsewhere", Timestamp: "9:00 AM"},
					{TS: "1600000001.000100", UserID: "U2", UserName: "bob", Text: "also", Timestamp: "9:01 AM"},
				})
			},
			rendered: []string{"1600000000.000100", "1600000001.000100"},
		},
		{
			name:     "older messages arrive",
			msgs:     func() []MessageItem { return slices.Clone(base[2:]) },
			event:    func(m *Model) { m.PrependMessages(slices.Clone(base[:2])) },
			rendered: []string{ts[0], ts[1]},
		},
		{
			name: "a sent message gets its ts",
			msgs: func() []MessageItem {
				return append(slices.Clone(base), MessageItem{TS: "local:1", UserID: "U1", UserName: "alice", Text: "sent"})
			},
			event: func(m *Model) {
				m.SwapLocalSent("local:1", MessageItem{TS: "1700400000.000100", UserID: "U1", UserName: "alice", Text: "sent", Timestamp: "11:00 AM"})
			},
			rendered: []string{"1700400000.000100"},
		},
		{
			name: "a send fails",
			msgs: func() []MessageItem {
				return append(slices.Clone(base), MessageItem{TS: "local:1", UserID: "U1", UserName: "alice", Text: "sent"})
			},
			event:    func(m *Model) { m.RemoveLocalSent("local:1") },
			rendered: []string{ts[6]},
		},
		{
			name: "the sent copy replaces the echo",
			event: func(m *Model) {
				m.UpsertSelfSent(MessageItem{TS: ts[5], UserID: "U2", UserName: "bob", Text: "line one\nline two", Timestamp: "10:30 AM"})
			},
			rendered: []string{ts[5]},
		},
		{
			name: "the sent copy lands first",
			event: func(m *Model) {
				m.UpsertSelfSent(MessageItem{TS: "1700400000.000100", UserID: "U2", UserName: "bob", Text: "sent", Timestamp: "11:00 AM"})
			},
			rendered: []string{ts[6], "1700400000.000100"},
		},
		{
			// Every entry's selected rows carry the focus of the build
			// that rendered it: an event under another focus renders
			// them all, as a dropped cache did.
			name: "a name after a focus change",
			event: func(m *Model) {
				m.SetFocused(false)
				_ = m.View(prH, prW)
				m.PatchUserName("U7", "gus")
			},
			rendered: ts,
		},
		{name: "the cursor moves up", event: func(m *Model) { m.MoveUp() }},
		{name: "the cursor jumps", event: func(m *Model) { m.SelectByIndex(1) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msgs := slices.Clone(base)
			if tc.msgs != nil {
				msgs = tc.msgs()
			}
			m := New(msgs, "general")
			m.SetFocused(true)
			if tc.setup != nil {
				tc.setup(m)
			}
			_ = m.View(prH, prW)
			before := slices.Clone(m.list.Entries())

			tc.event(m)
			_ = m.View(prH, prW)

			if got := rerendered(before, m.list.Entries()); !slices.Equal(got, tc.rendered) {
				t.Errorf("rendered %v again, want %v", got, tc.rendered)
			}
			assertMatchesFullRebuild(t, m)
		})
	}
}
