package ui

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/gammons/slk/internal/ui/wintree"
)

// A channel that opens on the fresh-cache path reads no history, so its
// pin count comes from a one-message history read of its own; a channel
// whose history read is on its way waits for that one.
func TestPinsBookmarks_FreshCacheOpenReadsPinCount(t *testing.T) {
	a, f, _ := pinsApp(t)
	a.setChannelSyncedAtReaderForTest(func(ids.ChannelID) int64 { return time.Now().Unix() - 5 })
	visitChannel(t, a, "C1", "checkout")
	if !slices.Contains(f.Calls(), "pincount C1") {
		t.Errorf("calls %v, want the pin count read", f.Calls())
	}
	if s := screenText(a); !strings.Contains(s, "📌 2 pins") {
		t.Errorf("count line on a fresh-cache open lacks the pins:\n%s", s)
	}
	a.setChannelSyncedAtReaderForTest(func(ids.ChannelID) int64 { return 0 })
	visitChannel(t, a, "C2", "payments")
	if slices.Contains(f.Calls(), "pincount C2") {
		t.Errorf("calls %v: C2's history read brings its count", f.Calls())
	}
}

// Slack sends no bookmark events, so the dialog reads both lists every
// time it opens.
func TestPinsBookmarks_DialogRereadsBothLists(t *testing.T) {
	a, f, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	_, cmd := a.Update(keyPress('p'))
	settle(t, a, cmd)
	a.Update(keyCode(tea.KeyEscape))
	before := len(f.Calls())
	_, cmd = a.Update(keyPress('p'))
	settle(t, a, cmd)
	got := f.Calls()[before:]
	slices.Sort(got)
	if want := []string{"bookmarks C1", "pins C1"}; !slices.Equal(got, want) {
		t.Errorf("reopening the dialog read %v, want %v", got, want)
	}
}

// A channel reopened more than five minutes after its bookmarks were
// read reads them again.
func TestPinsBookmarks_BookmarksRereadAfterFiveMinutes(t *testing.T) {
	a, f, _ := pinsApp(t)
	now := time.Now()
	a.SetNowFunc(func() time.Time { return now })
	count := func() int {
		n := 0
		for _, c := range f.Calls() {
			if c == "bookmarks C1" {
				n++
			}
		}
		return n
	}
	visitChannel(t, a, "C1", "checkout")
	visitChannel(t, a, "C2", "payments")
	now = now.Add(4 * time.Minute)
	visitChannel(t, a, "C1", "checkout")
	if count() != 1 {
		t.Errorf("reopened after 4 minutes: %d bookmark reads, want 1", count())
	}
	visitChannel(t, a, "C2", "payments")
	now = now.Add(2 * time.Minute)
	visitChannel(t, a, "C1", "checkout")
	if count() != 2 {
		t.Errorf("reopened 6 minutes after the read: %d bookmark reads, want 2", count())
	}
}

// The count line is styled when drawn, so a theme switch recolors it.
func TestPinsBookmarks_CountLineFollowsTheme(t *testing.T) {
	a, _, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2})
	row := a.pinsCountRow(a.activePinsKey())
	theme := styles.CurrentTheme()
	t.Cleanup(func() { a.applyTheme(theme) })
	a.applyTheme("light")
	if got, want := row(80), a.pinsCountRow(a.activePinsKey())(80); got != want {
		t.Errorf("a count line drawn after a theme switch keeps the old colors:\n got %q\nwant %q", got, want)
	}
}

// Every split window showing the channel gets its count line.
func TestPinsBookmarks_CountLineInEverySplitPane(t *testing.T) {
	a, _, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	_ = a.splitWindow(wintree.SplitSideBySide)
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 5})
	if n := strings.Count(screenText(a), "📌 5 pins"); n != 2 {
		t.Errorf("%d panes show the new count, want both:\n%s", n, screenText(a))
	}
}

// Focusing a window whose channel was never selected this session, as
// a restored one, reads its lists as a channel select does.
func TestPinsBookmarks_FocusingRestoredWindowReadsItsLists(t *testing.T) {
	a, f, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	first := a.focusedWin
	_ = a.splitWindow(wintree.SplitSideBySide)
	second := a.focusedWin
	settle(t, a, a.focusWindow(first))
	a.wins.SetChannel(second, wintree.Channel{ID: "C2", Name: "payments", Type: "channel"})
	settle(t, a, a.focusWindow(second))
	for _, want := range []string{"bookmarks C2", "pincount C2"} {
		if !slices.Contains(f.Calls(), want) {
			t.Errorf("calls %v lack %q", f.Calls(), want)
		}
	}
}

// A failed bookmarks read leaves the pin count on show and names only
// what failed, with the key that shows why.
func TestPinsBookmarks_BookmarksErrorKeepsPinCount(t *testing.T) {
	a, f, _ := pinsApp(t)
	f.bookmarksErr = errors.New("boom")
	visitChannel(t, a, "C1", "checkout")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2})
	s := screenText(a)
	for _, want := range []string{"📌 2 pins", "! Couldn't load bookmarks   B to see why and retry"} {
		if !strings.Contains(s, want) {
			t.Errorf("count line lacks %q:\n%s", want, s)
		}
	}
}

// No count line shows until there is something to show, so the pane
// does not jump.
func TestPinsBookmarks_NoCountLineWhileLoading(t *testing.T) {
	a, _, _ := pinsApp(t)
	a.Update(ChannelSelectedMsg{ID: "C1", Name: "checkout", Type: "channel"})
	if s := screenText(a); strings.Contains(s, "Loading pins and bookmarks") {
		t.Errorf("a loading count line shows:\n%s", s)
	}
}

// A channel left before its read is due reads nothing.
func TestPinsBookmarks_QuickWalkReadsOnlyWhereItStops(t *testing.T) {
	a, f, _ := pinsApp(t)
	_, c1 := a.Update(ChannelSelectedMsg{ID: "C1", Name: "checkout", Type: "channel"})
	_, c2 := a.Update(ChannelSelectedMsg{ID: "C2", Name: "payments", Type: "channel"})
	settle(t, a, c1)
	settle(t, a, c2)
	if calls := f.Calls(); slices.Contains(calls, "bookmarks C1") || !slices.Contains(calls, "bookmarks C2") {
		t.Errorf("calls %v, want C2's read and not C1's", calls)
	}
}

// A history read that began before a pin event cannot roll the count
// back.
func TestPinsBookmarks_HistoryCountFromBeforeAPinEventIgnored(t *testing.T) {
	a, _, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2, Started: time.Now()})
	started := time.Now()
	time.Sleep(time.Millisecond)
	a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C1", Added: true})
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2, Started: started})
	if s := screenText(a); !strings.Contains(s, "📌 3 pins") {
		t.Errorf("a count read before the pin event rolled the count back:\n%s", s)
	}
}

// The link picker marks a link to a channel of this workspace [slk], as
// o opens it in slk.
func TestLinkPicker_ChannelLinkGetsSlkBadge(t *testing.T) {
	a, _, _ := pinsApp(t)
	visitChannel(t, a, "C1", "checkout")
	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{
		{TS: "1700000000.000100", Text: "see <https://acme.slack.com/archives/C2|#payments> and <https://example.com|this>"},
	}})
	a.focusedPanel = PanelMessages
	a.messagepane.SelectByTS("1700000000.000100")
	a.Update(keyPress('o'))
	if !a.linkPicker.IsVisible() {
		t.Fatal("o on a message with two links opened no picker")
	}
	for _, it := range a.linkPicker.Items() {
		if want := strings.Contains(it.URL, "/archives/C2"); it.InApp != want {
			t.Errorf("%s: [slk] %v, want %v", it.URL, it.InApp, want)
		}
	}
}
