package thread

import (
	"fmt"
	"testing"
	"time"

	"github.com/gammons/slk/internal/ui/messages"
)

const memoH, memoW = 30, 90

// assertFrameMatchesUnmemoized fails when the frame m draws differs
// from the one it draws with its view memo dropped, the parent and the
// list built again.
func assertFrameMatchesUnmemoized(t *testing.T, m *Model, step string) {
	t.Helper()
	got := m.View(memoH, memoW)
	m.memo = viewMemo{}
	if want := m.View(memoH, memoW); got != want {
		t.Errorf("%s: frame differs from an unmemoized one\n got:\n%s\nwant:\n%s", step, got, want)
	}
}

// The parent's entry and the list are kept between frames; every change
// to what they show without an invalidation still reaches the frame.
func TestViewMemo_FollowsWhatTheParentShows(t *testing.T) {
	now := time.Date(2026, 11, 2, 12, 0, 0, 0, time.Local)
	messages.SetNowFunc(func() time.Time { return now })
	t.Cleanup(func() { messages.SetNowFunc(nil) })

	ts := func(d time.Time) string { return fmt.Sprintf("%d.000000", d.Unix()) }
	parent := messages.MessageItem{TS: ts(now.Add(-26 * time.Hour)), UserID: "U1", UserName: "alice", Text: "parent with `code`", Timestamp: "10:00 AM"}
	replies := []messages.MessageItem{
		{TS: ts(now.Add(-25 * time.Hour)), UserID: "U2", UserName: "bob", Text: "first", Timestamp: "11:00 AM"},
		{TS: ts(now.Add(-time.Hour)), UserID: "U2", UserName: "bob", Text: "second", Timestamp: "11:00 AM"},
	}
	m := New()
	m.SetThread(parent, replies, "C1", parent.TS)
	m.SetLastReadTS(replies[0].TS)
	m.SetFocused(true)
	assertFrameMatchesUnmemoized(t, m, "open")

	m.GoToTop()
	assertFrameMatchesUnmemoized(t, m, "cursor on the parent")
	m.SetFocused(false)
	assertFrameMatchesUnmemoized(t, m, "focus moves away")
	m.SetFocused(true)
	m.BeginSelectionAt(m.ChromeHeight(), 3)
	m.ExtendSelectionAt(m.ChromeHeight(), 12)
	assertFrameMatchesUnmemoized(t, m, "text selection on the parent")
	m.ClearSelection()
	assertFrameMatchesUnmemoized(t, m, "text selection cleared")
	m.MoveDown()
	assertFrameMatchesUnmemoized(t, m, "cursor back on a reply")
	m.SetLastReadTS(replies[1].TS)
	assertFrameMatchesUnmemoized(t, m, "unread boundary moves")
	now = now.Add(24 * time.Hour)
	assertFrameMatchesUnmemoized(t, m, "the day turns")
}

func TestShowsMessage(t *testing.T) {
	m := newThreadWithReplies(1)
	_ = m.View(memoH, memoW)
	for ts, want := range map[string]bool{"100.0": true, "101.0": true, "999.0": false, "": false} {
		if got := m.ShowsMessage(ts); got != want {
			t.Errorf("ShowsMessage(%q) = %v, want %v", ts, got, want)
		}
	}
}
