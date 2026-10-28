package messages

import (
	"fmt"
	stdimage "image"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	emojiutil "github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
)

const prH, prW = 400, 80 // tall enough to show every message

// prMessages returns n messages, two per day, so the list carries day
// dividers between some neighbours and not others.
func prMessages(n int) []MessageItem {
	msgs := make([]MessageItem, n)
	for i := range msgs {
		msgs[i] = MessageItem{
			TS:        fmt.Sprintf("%d.000100", 1700000000+(i/2)*86400+i),
			UserName:  []string{"alice", "bob"}[i%2],
			UserID:    []string{"U1", "U2"}[i%2],
			Text:      fmt.Sprintf("message %d %s", i, strings.Repeat("word ", 3*(i%4))),
			Timestamp: "10:30 AM",
		}
	}
	return msgs
}

// rerendered reports the IDs of the message entries whose rows were
// rendered again since before: a kept entry shares its rows.
func rerendered(before, after []entrylist.Entry) []string {
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
// discarded cache, and fails unless the view and every entry match.
func assertMatchesFullRebuild(t *testing.T, m *Model) {
	t.Helper()
	got := m.View(prH, prW)
	gotEntries := slices.Clone(m.list.Entries())
	m.InvalidateCache()
	want := m.View(prH, prW)
	if got != want {
		t.Errorf("view differs from a full rebuild\n got: %q\nwant: %q", got, want)
	}
	if !reflect.DeepEqual(gotEntries, m.list.Entries()) {
		t.Error("entries differ from a full rebuild")
	}
}

// Every event the channel pane handles in place must leave the pane
// exactly as a full rebuild would, rendering only the entries it changed.
func TestModel_PartialRebuildMatchesFullRebuild(t *testing.T) {
	msgs := prMessages(7)
	cases := []struct {
		name       string
		lastReadTS string
		event      func(m *Model)
		rendered   []string // entries rendered again by the event's View
	}{
		{
			name:     "first reaction",
			event:    func(m *Model) { m.UpdateReaction(msgs[3].TS, "eyes", "U9", false) },
			rendered: []string{msgs[3].TS},
		},
		{
			name: "reaction removed",
			event: func(m *Model) {
				m.UpdateReaction(msgs[3].TS, "eyes", "U9", false)
				_ = m.View(prH, prW)
				m.UpdateReaction(msgs[3].TS, "eyes", "U9", true)
			},
			rendered: []string{msgs[3].TS},
		},
		{
			name: "edit",
			event: func(m *Model) {
				m.UpdateMessageInPlace(MessageItem{TS: msgs[2].TS, Text: strings.Repeat("edited ", 30), IsEdited: true})
			},
			rendered: []string{msgs[2].TS},
		},
		{
			name:     "first reply",
			event:    func(m *Model) { m.IncrementReplyCount(msgs[4].TS, "1800000000.000001") },
			rendered: []string{msgs[4].TS},
		},
		{
			name: "append the same day",
			event: func(m *Model) {
				m.AppendMessage(MessageItem{TS: "1700259200.000200", UserName: "carol", Text: "same day", Timestamp: "11:00 AM"})
			},
			rendered: []string{msgs[6].TS, "1700259200.000200"},
		},
		{
			name: "append on a new day",
			event: func(m *Model) {
				m.AppendMessage(MessageItem{TS: "1700400000.000100", UserName: "carol", Text: "next day", Timestamp: "11:00 AM"})
			},
			rendered: []string{msgs[6].TS, "1700400000.000100"},
		},
		{
			name:       "append the first unread",
			lastReadTS: msgs[6].TS,
			event: func(m *Model) {
				m.AppendMessage(MessageItem{TS: "1700259200.000200", UserName: "carol", Text: "unread", Timestamp: "11:00 AM"})
			},
			rendered: []string{msgs[6].TS, "1700259200.000200"},
		},
		{
			name:       "append after the new divider",
			lastReadTS: msgs[5].TS,
			event: func(m *Model) {
				m.AppendMessage(MessageItem{TS: "1700259200.000200", UserName: "carol", Text: "unread", Timestamp: "11:00 AM"})
			},
			rendered: []string{msgs[6].TS, "1700259200.000200"},
		},
		{
			name:     "remove from the middle",
			event:    func(m *Model) { m.RemoveMessageByTS(msgs[3].TS) },
			rendered: nil,
		},
		{
			name:     "remove the first of a day",
			event:    func(m *Model) { m.RemoveMessageByTS(msgs[2].TS) },
			rendered: nil,
		},
		{
			name:     "remove the only one of a day",
			event:    func(m *Model) { m.RemoveMessageByTS(msgs[6].TS) },
			rendered: []string{msgs[5].TS},
		},
		{
			name:       "remove the first unread",
			lastReadTS: msgs[3].TS,
			event:      func(m *Model) { m.RemoveMessageByTS(msgs[4].TS) },
			rendered:   nil,
		},
		{
			name: "append, react and remove before a View",
			event: func(m *Model) {
				m.AppendMessage(MessageItem{TS: "1700400000.000100", UserName: "carol", Text: "next day", Timestamp: "11:00 AM"})
				m.UpdateReaction(msgs[1].TS, "eyes", "U9", false)
				m.RemoveMessageByTS(msgs[0].TS)
			},
			rendered: []string{msgs[1].TS, msgs[6].TS, "1700400000.000100"},
		},
		{
			// The entries' selected rows carry the focus of the build
			// that rendered them; SetFocused re-renders only the
			// selected one. An event under the other focus rebuilds
			// all of them, so a later cursor move draws the new focus.
			name: "reaction after a focus change",
			event: func(m *Model) {
				m.SetFocused(false)
				_ = m.View(prH, prW)
				m.UpdateReaction(msgs[3].TS, "eyes", "U9", false)
				_ = m.View(prH, prW)
				m.MoveUp()
			},
			rendered: tsOf(msgs),
		},
		{
			// The selected entry draws the reaction-nav highlight, and
			// the append moves the selection off it.
			name: "append in reaction nav",
			event: func(m *Model) {
				m.UpdateReaction(msgs[3].TS, "eyes", "U9", false)
				m.SelectByIndex(3)
				_ = m.View(prH, prW)
				m.EnterReactionNav()
				_ = m.View(prH, prW)
				m.AppendMessage(MessageItem{TS: "1700259200.000200", UserName: "carol", Text: "new", Timestamp: "11:00 AM"})
			},
			rendered: append(tsOf(msgs), "1700259200.000200"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New(slices.Clone(msgs), "general")
			m.SetFocused(true)
			m.SetLastReadTS(tc.lastReadTS)
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

func tsOf(msgs []MessageItem) []string {
	ts := make([]string, len(msgs))
	for i, m := range msgs {
		ts[i] = m.TS
	}
	return ts
}

// An emoji image landing re-renders only the entries that drew one of
// their emoji as a cold reservation.
func TestModel_HandleEmojiImageReady_RerendersColdEntries(t *testing.T) {
	emojiutil.SetImageMode(true, 2)
	t.Cleanup(func() { emojiutil.SetImageMode(false, 2) })

	thumbURL := emojiutil.CDNBaseURL + "1f44d.png"
	ff := newFakePlaceFetcher() // defined in render_test.go
	msgs := prMessages(5)
	msgs[1].Text = "cold :thumbsup:"
	msgs[3].Reactions = []ReactionItem{{Emoji: "thumbsup", Count: 1, UserIDs: []string{"U1"}}}
	m := New(msgs, "general")
	m.SetEmojiContext(EmojiContext{PlaceCtx: emojiutil.PlaceContext{Fetcher: ff}, Cells: 2})
	_ = m.View(prH, prW)

	ff.setPrerendered(emojiutil.EmojiCacheKey(thumbURL), stdimage.Pt(2, 1), imgpkg.Render{
		Cells: stdimage.Pt(2, 1),
		Lines: []string{"\U0010EEEE\U0010EEEE"},
	})
	before := slices.Clone(m.list.Entries())
	m.HandleEmojiImageReady(thumbURL)
	view := m.View(prH, prW)

	if !strings.Contains(view, "\U0010EEEE") {
		t.Error("the landed emoji image is not drawn")
	}
	want := []string{msgs[1].TS, msgs[3].TS}
	if got := rerendered(before, m.list.Entries()); !slices.Equal(got, want) {
		t.Errorf("rendered %v again, want %v", got, want)
	}
	assertMatchesFullRebuild(t, m)

	// Now warm: another landing renders nothing.
	before = slices.Clone(m.list.Entries())
	m.HandleEmojiImageReady("")
	_ = m.View(prH, prW)
	if got := rerendered(before, m.list.Entries()); len(got) != 0 {
		t.Errorf("rendered %v again with every emoji warm, want none", got)
	}
}
