package thread

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/peerstatus"
)

const orH, orW = 200, 80 // tall enough to show the whole thread

// orThread returns a parent by U1 and five replies: U2 wrote the even
// ones, U3 the odd ones; reply 1 mentions U7 in its text, reply 2 U8
// in a rich_text block only; the parent and the last reply have
// reactions.
func orThread(t *testing.T) (messages.MessageItem, []messages.MessageItem) {
	t.Helper()
	var p struct {
		Blocks slack.Blocks `json:"blocks"`
	}
	raw := `{"blocks":[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[` +
		`{"type":"text","text":"ping "},{"type":"user","user_id":"U8"}]}]}]}`
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	parent := messages.MessageItem{
		TS: "1700000000.000100", UserID: "U1", UserName: "alice", Text: "parent", Timestamp: "10:00 AM",
		Reactions: []messages.ReactionItem{{Emoji: "eyes", Count: 1}},
	}
	replies := make([]messages.MessageItem, 5)
	for i := range replies {
		replies[i] = messages.MessageItem{
			TS:        fmt.Sprintf("%d.000100", 1700000001+i),
			UserID:    []string{"U2", "U3"}[i%2],
			UserName:  []string{"bob", "carol"}[i%2],
			Text:      fmt.Sprintf("reply %d", i),
			Timestamp: "10:30 AM",
			ThreadTS:  parent.TS,
		}
	}
	replies[1].Text = "hi <@U7>"
	replies[2].Text, replies[2].Blocks = "ping", blockkit.Parse(p.Blocks)
	replies[4].Reactions = []messages.ReactionItem{{Emoji: "eyes", Count: 1}, {Emoji: "tada", Count: 2}}
	return parent, replies
}

// orRerendered reports the IDs of the entries (parent and replies) whose
// rows were rendered again since before: a kept entry shares its rows.
func orRerendered(before, after []entrylist.Entry) []string {
	kept := make(map[*string]bool, len(before))
	for _, e := range before {
		if len(e.Lines) > 0 {
			kept[&e.Lines[0]] = true
		}
	}
	var ids []string
	for _, e := range after {
		if e.ID != "" && !kept[&e.Lines[0]] {
			ids = append(ids, e.ID)
		}
	}
	return ids
}

// assertMatchesFullRebuild renders m, then renders it again from a
// discarded cache, and fails unless the frame, every entry and the rows
// each reply draws when the cursor moves to it match.
func assertMatchesFullRebuild(t *testing.T, m *Model) {
	t.Helper()
	got := m.View(orH, orW)
	gotEntries := slices.Clone(m.list.Entries())
	gotSelected := allReplySelectedRows(m)
	m.InvalidateCache()
	want := m.View(orH, orW)
	if got != want {
		t.Errorf("frame differs from a full rebuild\n got: %q\nwant: %q", got, want)
	}
	if !reflect.DeepEqual(gotEntries, m.list.Entries()) {
		t.Error("entries differ from a full rebuild")
	}
	if !reflect.DeepEqual(gotSelected, allReplySelectedRows(m)) {
		t.Error("selected rows differ from a full rebuild")
	}
}

// allReplySelectedRows returns the rows each reply of m draws as the
// cursor's.
func allReplySelectedRows(m *Model) [][]string {
	rows := make([][]string, len(m.replyEntries))
	for i := range rows {
		rows[i] = replySelectedRows(m, i)
	}
	return rows
}

// Every event the thread pane handles without a full rebuild leaves it
// exactly as a full rebuild would, rendering only the parent and replies
// it changed. A dependency an event misses shows here as a row that
// differs from the full rebuild's.
func TestThread_TargetedInvalidationMatchesFullRebuild(t *testing.T) {
	parent, base := orThread(t)
	ts := make([]string, len(base))
	for i, r := range base {
		ts[i] = r.TS
	}
	later := time.Now().Add(time.Hour)
	reloaded := func(edit func([]messages.MessageItem) []messages.MessageItem) func(m *Model) {
		return func(m *Model) {
			replies := slices.Clone(base)
			if edit != nil {
				replies = edit(replies)
			}
			m.SetThread(parent, replies, "C1", parent.TS)
		}
	}
	cases := []struct {
		name     string
		setup    func(m *Model) // before the first View
		event    func(m *Model)
		rendered []string // parent and replies rendered again by the event's View
	}{
		{name: "the parent author's name", event: func(m *Model) { m.PatchUserName("U1", "alicia") }, rendered: []string{parent.TS}},
		{name: "a reply author's name", event: func(m *Model) { m.PatchUserName("U3", "cara") }, rendered: []string{ts[1], ts[3]}},
		{name: "a name mentioned in text", event: func(m *Model) { m.PatchUserName("U7", "gus") }, rendered: []string{ts[1]}},
		{name: "a name mentioned in a block", event: func(m *Model) { m.PatchUserName("U8", "hal") }, rendered: []string{ts[2]}},
		{name: "a name nobody here uses", event: func(m *Model) { m.PatchUserName("U99", "zed") }},
		{
			name:     "a reply author's status",
			event:    func(m *Model) { m.PatchUserStatus("U2", peerstatus.Status{Emoji: ":calendar:"}) },
			rendered: []string{ts[0], ts[2], ts[4]},
		},
		{
			name:     "the parent author's status",
			event:    func(m *Model) { m.PatchUserStatus("U1", peerstatus.Status{Emoji: ":calendar:", Expires: later}) },
			rendered: []string{parent.TS},
		},
		{name: "reaction nav starts", event: func(m *Model) { m.EnterReactionNav() }, rendered: []string{ts[4]}},
		{
			name:     "reaction nav right",
			setup:    func(m *Model) { m.EnterReactionNav() },
			event:    func(m *Model) { m.ReactionNavRight() },
			rendered: []string{ts[4]},
		},
		{
			name:     "reaction nav ends",
			setup:    func(m *Model) { m.EnterReactionNav() },
			event:    func(m *Model) { m.ExitReactionNav() },
			rendered: []string{ts[4]},
		},
		{
			name:     "reaction nav on the parent",
			setup:    func(m *Model) { m.GoToTop() },
			event:    func(m *Model) { m.EnterReactionNav(); _ = m.View(orH, orW); m.ReactionNavRight() },
			rendered: []string{parent.TS},
		},
		{
			name:     "a reaction on a reply",
			event:    func(m *Model) { m.UpdateReaction(ts[2], "eyes", "U9", false) },
			rendered: []string{ts[2]},
		},
		{
			name:     "a reaction on the parent",
			event:    func(m *Model) { m.UpdateReaction(parent.TS, "tada", "U9", false) },
			rendered: []string{parent.TS},
		},
		{
			name: "an edit",
			event: func(m *Model) {
				m.UpdateMessageInPlace(messages.MessageItem{TS: ts[3], Text: "edited", IsEdited: true})
			},
			rendered: []string{ts[3]},
		},
		{
			name: "the parent edited",
			event: func(m *Model) {
				m.UpdateParentInPlace(messages.MessageItem{TS: parent.TS, Text: "parent, edited", IsEdited: true})
			},
			rendered: []string{parent.TS},
		},
		{
			name: "a reply arrives",
			event: func(m *Model) {
				m.AddReply(messages.MessageItem{TS: "1700000010.000100", UserID: "U2", UserName: "bob", Text: "new", Timestamp: "10:40 AM"})
			},
			rendered: []string{"1700000010.000100"},
		},
		{name: "a reply deleted", event: func(m *Model) { m.RemoveMessageByTS(ts[2]) }},
		{
			name: "a sent reply gets its ts",
			setup: func(m *Model) {
				m.AddReply(messages.MessageItem{TS: "local:1", UserID: "U2", UserName: "bob", Text: "sent"})
			},
			event: func(m *Model) {
				m.SwapLocalSentReply("local:1", messages.MessageItem{TS: "1700000010.000100", UserID: "U2", UserName: "bob", Text: "sent", Timestamp: "10:40 AM"})
			},
			rendered: []string{"1700000010.000100"},
		},
		{
			name: "a reply send fails",
			setup: func(m *Model) {
				m.AddReply(messages.MessageItem{TS: "local:1", UserID: "U2", UserName: "bob", Text: "sent"})
			},
			event: func(m *Model) { m.RemoveLocalSentReply("local:1") },
		},
		{
			name: "the sent copy replaces the echo",
			event: func(m *Model) {
				m.UpsertSelfSentReply(messages.MessageItem{TS: ts[4], UserID: "U2", UserName: "bob", Text: "line one\nline two", Timestamp: "10:30 AM"})
			},
			rendered: []string{ts[4]},
		},
		{name: "the fetch lands unchanged", event: reloaded(nil)},
		{
			name: "the fetch lands with an edit and a new reply",
			event: reloaded(func(r []messages.MessageItem) []messages.MessageItem {
				r[0].Text = "edited"
				return append(r, messages.MessageItem{TS: "1700000010.000100", UserID: "U3", UserName: "carol", Text: "new", Timestamp: "10:40 AM"})
			}),
			rendered: []string{ts[0], "1700000010.000100"},
		},
		{
			name: "the fetch lands with the parent edited",
			event: func(m *Model) {
				p := parent
				p.Text, p.IsEdited = "parent, edited", true
				m.SetThread(p, slices.Clone(base), "C1", parent.TS)
			},
			rendered: []string{parent.TS},
		},
		{
			name: "another thread",
			event: func(m *Model) {
				m.SetThread(messages.MessageItem{TS: "1600000000.000100", UserID: "U1", UserName: "alice", Text: "other", Timestamp: "9:00 AM"},
					[]messages.MessageItem{{TS: "1600000001.000100", UserID: "U2", UserName: "bob", Text: "r", Timestamp: "9:01 AM"}}, "C1", "1600000000.000100")
			},
			rendered: []string{"1600000000.000100", "1600000001.000100"},
		},
		{
			// The replies' selected rows carry the focus of the build that
			// rendered them: an event under another focus renders them
			// all, as a dropped cache did.
			name: "a name after a focus change",
			event: func(m *Model) {
				m.SetFocused(false)
				_ = m.View(orH, orW)
				m.PatchUserName("U7", "gus")
			},
			rendered: append([]string{parent.TS}, ts...),
		},
		{name: "the cursor moves up", event: func(m *Model) { m.MoveUp() }},
		{
			name: "the cursor visits every reply",
			event: func(m *Model) {
				for range base[1:] { // up to the first reply
					m.MoveUp()
					_ = m.View(orH, orW)
				}
				m.GoToBottom()
			},
		},
		{
			name: "an edit of a reply the cursor visited",
			event: func(m *Model) {
				m.MoveUp()
				_ = m.View(orH, orW)
				m.MoveDown()
				_ = m.View(orH, orW)
				m.UpdateMessageInPlace(messages.MessageItem{TS: ts[3], Text: "edited", IsEdited: true})
			},
			rendered: []string{ts[3]},
		},
		{name: "the cursor moves to the parent", event: func(m *Model) { m.GoToTop() }, rendered: []string{parent.TS}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			m.SetThread(parent, slices.Clone(base), "C1", parent.TS)
			m.SetFocused(true)
			if tc.setup != nil {
				tc.setup(m)
			}
			_ = m.View(orH, orW)
			before := slices.Clone(m.list.Entries())

			tc.event(m)
			_ = m.View(orH, orW)

			if got := orRerendered(before, m.list.Entries()); !slices.Equal(got, tc.rendered) {
				t.Errorf("rendered %v again, want %v", got, tc.rendered)
			}
			assertMatchesFullRebuild(t, m)
		})
	}
}
