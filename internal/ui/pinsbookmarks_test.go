package ui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/pinsbookmarks"
	"github.com/gammons/slk/internal/ui/wintree"
)

// fakePinsBookmarks answers from its maps, fails a list with its error,
// and records each call as "pins C1" or "bookmarks C1". With together
// set, a pins call waits for the bookmarks call, as only a concurrent
// read of the two lets it.
type fakePinsBookmarks struct {
	mu                    sync.Mutex
	pins                  map[string][]pinsbookmarks.Pin
	bookmarks             map[string][]pinsbookmarks.Bookmark
	pinsErr, bookmarksErr error
	calls                 []string
	together              chan struct{}
}

func (f *fakePinsBookmarks) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakePinsBookmarks) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakePinsBookmarks) Pins(_ context.Context, _, channelID string) ([]pinsbookmarks.Pin, error) {
	f.record("pins " + channelID)
	if f.together != nil {
		select {
		case <-f.together:
		case <-time.After(5 * time.Second):
			return nil, errors.New("bookmarks were not read alongside")
		}
	}
	return f.pins[channelID], f.pinsErr
}

func (f *fakePinsBookmarks) Bookmarks(_ context.Context, _, channelID string) ([]pinsbookmarks.Bookmark, error) {
	f.record("bookmarks " + channelID)
	if f.together != nil {
		close(f.together)
	}
	return f.bookmarks[channelID], f.bookmarksErr
}

// PinCount answers with how many pins its map holds, recorded as
// "pincount C1".
func (f *fakePinsBookmarks) PinCount(_ context.Context, _, channelID string) (int, error) {
	f.record("pincount " + channelID)
	return len(f.pins[channelID]), nil
}

// settle runs cmd, its batches' members included, and feeds what the
// pins and bookmarks code sends back through Update, and so on, until
// nothing more comes.
func settle(t *testing.T, a *App, cmd tea.Cmd) {
	t.Helper()
	for _, m := range drainCmd(cmd) {
		switch m.(type) {
		case pinsBookmarksLoadedMsg, pinsReadDueMsg, PinCountMsg:
			_, next := a.Update(m)
			settle(t, a, next)
		}
	}
}

// visitChannel opens channel id through Update and settles every read
// that starts.
func visitChannel(t *testing.T, a *App, id, name string) {
	t.Helper()
	_, cmd := a.Update(ChannelSelectedMsg{ID: id, Name: name, Type: "channel"})
	settle(t, a, cmd)
}

const (
	pinTS       = "1700000000.000100"
	pinReplyTS  = "1700000500.000300"
	pinThreadTS = "1700000400.000200"
)

// pinsApp is a 160x40 App in workspace T1 (acme.slack.com) whose
// channels are C1 #checkout, with two pins (a message and a thread
// reply) and four bookmarks (the web, a message in #payments, #payments
// itself and a canvas), and C2 #payments, with neither. The browser
// opener records what it was given.
func pinsApp(t *testing.T) (*App, *fakePinsBookmarks, *string) {
	t.Helper()
	a := newTestApp(t, withSize(160, 40), withActiveTeam("T1"))
	a.workspaceDomains["T1"] = "acme"
	var opened string
	a.browserOpener = func(url string) tea.Cmd {
		opened = url
		return nil
	}
	a.setChannelLookupFuncForTest(func(id ids.ChannelID) (string, string, bool) {
		switch id {
		case "C1":
			return "checkout", "channel", true
		case "C2":
			return "payments", "channel", true
		}
		return "", "", false
	})
	f := &fakePinsBookmarks{
		pins: map[string][]pinsbookmarks.Pin{"C1": {
			{TS: pinTS, UserID: "U1", Username: "maya", Text: "Rollback steps are here",
				Permalink: "https://acme.slack.com/archives/C1/p1700000000000100"},
			{TS: pinReplyTS, ThreadTS: pinThreadTS, Username: "sam", Text: "Root cause: the pool ran dry",
				Permalink: "https://acme.slack.com/archives/C1/p1700000500000300?thread_ts=1700000400.000200&cid=C1"},
		}},
		bookmarks: map[string][]pinsbookmarks.Bookmark{"C1": {
			{Title: "Runbook", Link: "https://wiki.acme.com/runbook"},
			{Title: "The decision", Link: "https://acme.slack.com/archives/C2/p1700000000000100"},
			{Title: "#payments", Link: "https://acme.slack.com/archives/C2"},
			{Title: "SEV2 canvas", Link: "https://acme.slack.com/docs/T1/F07QK2M9HD1"},
		}},
	}
	a.SetPinsBookmarksService(f)
	delay := pinsReadDelay
	pinsReadDelay = time.Millisecond
	t.Cleanup(func() { pinsReadDelay = delay })
	return a, f, &opened
}

// loadedMsgs runs cmd as the runtime does, the members of a batch
// concurrently, and returns the n list answers it delivers.
func loadedMsgs(t *testing.T, cmd tea.Cmd, n int) []pinsBookmarksLoadedMsg {
	t.Helper()
	ch := make(chan pinsBookmarksLoadedMsg, n)
	go deliverAll(cmd, ch)
	var out []pinsBookmarksLoadedMsg
	for range n {
		select {
		case m := <-ch:
			out = append(out, m)
		case <-time.After(10 * time.Second):
			t.Fatalf("got %d list answers, want %d", len(out), n)
		}
	}
	return out
}

func deliverAll(cmd tea.Cmd, ch chan<- pinsBookmarksLoadedMsg) {
	if cmd == nil {
		return
	}
	switch m := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range m {
			go deliverAll(c, ch)
		}
	case pinsBookmarksLoadedMsg:
		ch <- m
	}
}

// land runs cmd and feeds its n list answers back through Update,
// returning the commands they produced.
func land(t *testing.T, a *App, cmd tea.Cmd, n int) []tea.Cmd {
	t.Helper()
	var out []tea.Cmd
	for _, m := range loadedMsgs(t, cmd, n) {
		_, next := a.Update(m)
		out = append(out, next)
	}
	return out
}

// openChannel opens channel id through Update, settles its reads, and
// reports whether they read its bookmarks.
func openChannel(t *testing.T, a *App, id, name string) bool {
	t.Helper()
	f := a.pinsBookmarks.svc.(*fakePinsBookmarks)
	reads := func() int {
		n := 0
		for _, c := range f.Calls() {
			if c == "bookmarks "+id {
				n++
			}
		}
		return n
	}
	before := reads()
	visitChannel(t, a, id, name)
	return reads() > before
}

func TestPinsBookmarks_CountLine(t *testing.T) {
	a, f, _ := pinsApp(t)
	if !openChannel(t, a, "C1", "checkout") {
		t.Fatal("opening C1 read nothing")
	}
	if s := screenText(a); !strings.Contains(s, "🔖 4 bookmarks") || strings.Contains(s, "📌") {
		t.Errorf("before its history says, the count line has only bookmarks:\n%s", s)
	}
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2})
	s := screenText(a)
	for _, want := range []string{"📌 2 pins   🔖 4 bookmarks", "p pins  B bookmarks"} {
		if !strings.Contains(s, want) {
			t.Errorf("count line lacks %q:\n%s", want, s)
		}
	}
	openChannel(t, a, "C2", "payments")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C2", Count: 0})
	if s := screenText(a); strings.Contains(s, "📌") || strings.Contains(s, "B bookmarks") {
		t.Errorf("a channel with neither shows a count line:\n%s", s)
	}
	if openChannel(t, a, "C1", "checkout") {
		t.Error("opening C1 again read its bookmarks again")
	}
	if want := []string{"bookmarks C1", "bookmarks C2"}; !slices.Equal(f.Calls(), want) {
		t.Errorf("calls %v, want %v: pins.list waits for the dialog", f.Calls(), want)
	}
	if s := screenText(a); !strings.Contains(s, "📌 2 pins") {
		t.Errorf("the count line of C1 is gone on its return:\n%s", s)
	}
}

func TestPinsBookmarks_FailedBookmarksReadAgainOnNextOpen(t *testing.T) {
	a, f, _ := pinsApp(t)
	f.bookmarksErr = errors.New("dial tcp: no route to host")
	visitChannel(t, a, "C1", "checkout")
	if s := screenText(a); !strings.Contains(s, "! Couldn't load bookmarks   B to see why and retry") {
		t.Errorf("count line after a failure:\n%s", s)
	}
	f.bookmarksErr = nil
	openChannel(t, a, "C2", "payments")
	if !openChannel(t, a, "C1", "checkout") {
		t.Fatal("a failed read was not tried again on the next open")
	}
	if s := screenText(a); !strings.Contains(s, "🔖 4 bookmarks") {
		t.Errorf("count line after the second read:\n%s", s)
	}
}

func TestPinsBookmarks_DialogReadsBothListsAtOnce(t *testing.T) {
	a, f, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	f.together = make(chan struct{})
	_, cmd := a.Update(keyPress('p'))
	land(t, a, cmd, 2)
	s := screenText(a)
	if !strings.Contains(s, "Rollback steps are here") || !strings.Contains(s, "Runbook") {
		t.Errorf("dialog after both reads:\n%s", s)
	}
}

func TestPinsBookmarks_OneListFailsRetryReadsOnlyIt(t *testing.T) {
	a, f, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	f.pinsErr = &core.RateLimitedError{RetryAfter: 3 * time.Second}
	_, cmd := a.Update(keyPress('p'))
	land(t, a, cmd, 2)
	s := screenText(a)
	for _, want := range []string{"Slack is limiting requests right now. Wait 3 seconds, then press r to retry.", "Runbook"} {
		if !strings.Contains(s, want) {
			t.Errorf("dialog lacks %q:\n%s", want, s)
		}
	}
	f.pinsErr = nil
	before := len(f.Calls())
	_, cmd = a.Update(keyPress('r'))
	land(t, a, cmd, 1)
	if got := f.Calls()[before:]; !slices.Equal(got, []string{"pins C1"}) {
		t.Errorf("r read %v, want only the failed pins", got)
	}
	if s := screenText(a); !strings.Contains(s, "Rollback steps are here") {
		t.Errorf("after a good retry:\n%s", s)
	}
}

func TestPinsBookmarks_ErrText(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&core.RateLimitedError{RetryAfter: 3 * time.Second}, "Slack is limiting requests right now. Wait 3 seconds, then press r to retry."},
		{&core.RateLimitedError{}, "Slack is limiting requests right now. Wait a few seconds, then press r to retry."},
		{errors.New("pins.list: channel_not_found"), "Loading failed: pins.list: channel_not_found. Press r to retry."},
	} {
		if got := pinsBookmarksErrText(tc.err); got != tc.want {
			t.Errorf("%v: %q, want %q", tc.err, got, tc.want)
		}
	}
}

func TestPinsBookmarks_PinEvents(t *testing.T) {
	a, f, _ := pinsApp(t)
	openChannel(t, a, "C2", "payments")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C2", Count: 1})
	openChannel(t, a, "C1", "checkout")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 2})

	// The dialog closed: an event counts into the count line, no read.
	if _, cmd := a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C1", Added: true}); cmd != nil {
		t.Error("a pin event with the dialog closed read pins")
	}
	if s := screenText(a); !strings.Contains(s, "📌 3 pins") {
		t.Errorf("count line after a pin was added:\n%s", s)
	}
	if _, cmd := a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C2"}); cmd != nil {
		t.Error("a pin event for a channel off screen read pins")
	}
	if _, cmd := a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C9", Added: true}); cmd != nil {
		t.Error("a pin event for a channel never opened read pins")
	}

	// The dialog open: one read for the events of one in flight.
	_, cmd := a.Update(keyPress('p'))
	before := len(f.Calls())
	_, again := a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C1"})
	if again != nil {
		t.Error("an event while pins are read started another read")
	}
	a.Update(PinsChangedMsg{TeamID: "T1", ChannelID: "C1"})
	var next tea.Cmd
	for _, c := range land(t, a, cmd, 2) { // the pins and the bookmarks
		if c != nil {
			next = c
		}
	}
	if next == nil {
		t.Fatal("pins that changed while read were not read again")
	}
	if more := land(t, a, next, 1); more[0] != nil {
		t.Error("a read with no change since went on reading")
	}
	var pinReads []string
	for _, c := range f.Calls()[before:] {
		if strings.HasPrefix(c, "pins ") {
			pinReads = append(pinReads, c)
		}
	}
	if !slices.Equal(pinReads, []string{"pins C1", "pins C1"}) {
		t.Errorf("the dialog's read and two events cost %v, want two pin reads", pinReads)
	}
	if s := screenText(a); !strings.Contains(s, "📌 2 pins") {
		t.Errorf("count line after the reads, which count the pins:\n%s", s)
	}
}

func TestPinsBookmarks_KeysAndModes(t *testing.T) {
	a, f, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	openDialog(t, a, 'p')
	if a.mode != ModePinsBookmarks || !strings.Contains(statusbarText(a), "PINS") {
		t.Fatalf("p: mode %v, status bar %q", a.mode, statusbarText(a))
	}
	if !strings.Contains(screenText(a), "Pins and bookmarks · #checkout") {
		t.Errorf("the dialog does not name the channel:\n%s", screenText(a))
	}
	a.Update(keyCode(tea.KeyTab))
	if !strings.Contains(statusbarText(a), "BOOKMARKS") {
		t.Errorf("tab: status bar %q", statusbarText(a))
	}
	a.Update(keyCode(tea.KeyEscape))
	if a.mode != ModeNormal || a.pinsBookmarks.model.IsVisible() {
		t.Errorf("esc: mode %v, visible %v", a.mode, a.pinsBookmarks.model.IsVisible())
	}
	openDialog(t, a, 'B')
	if !strings.Contains(statusbarText(a), "BOOKMARKS") {
		t.Errorf("B: status bar %q", statusbarText(a))
	}
	a.SetMode(ModeNormal)
	if a.pinsBookmarks.model.IsVisible() {
		t.Error("leaving the mode by another path left the dialog drawn")
	}
	for cmd, want := range map[string]string{"pins": "PINS", "bookmarks": "BOOKMARKS"} {
		settle(t, a, executeCommand(a, cmd))
		if a.mode != ModePinsBookmarks || !strings.Contains(statusbarText(a), want) {
			t.Errorf(":%s: mode %v, status bar %q", cmd, a.mode, statusbarText(a))
		}
		a.SetMode(ModeNormal)
	}

	// B in a channel without bookmarks puts the cursor, and the status
	// bar, on its pins.
	f.bookmarks["C1"] = nil
	openDialog(t, a, 'B')
	if !strings.Contains(statusbarText(a), "PINS") {
		t.Errorf("B without bookmarks: status bar %q", statusbarText(a))
	}
}

// openDialog opens the dialog with key k and settles its reads.
func openDialog(t *testing.T, a *App, k rune) {
	t.Helper()
	_, cmd := a.Update(keyPress(k))
	settle(t, a, cmd)
}

func TestPinsBookmarks_OpenPinInChannel(t *testing.T) {
	a, _, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{{TS: pinTS, Text: "Rollback steps are here", ReplyCount: 3}, {TS: "1700000900.000000", Text: "newer"}}})
	openDialog(t, a, 'p')
	pressKey(t, a, keyCode(tea.KeyEnter))
	if sel, ok := a.messagepane.SelectedMessage(); !ok || sel.TS != pinTS {
		t.Errorf("selected %+v, want the pin %s", sel, pinTS)
	}
	if a.threadVisible {
		t.Error("a pinned thread parent opened its thread; Slack goes to the message")
	}
	if a.mode != ModeNormal || a.pinsBookmarks.model.IsVisible() {
		t.Error("opening a pin should close the dialog")
	}
}

func TestPinsBookmarks_OpenPinOlderThanLoaded(t *testing.T) {
	a, _, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	var around string
	setChannelFetchAroundForTest(a, func(channelID ids.ChannelID, ts ids.MessageTS) core.Msg {
		around = string(channelID) + "/" + string(ts)
		return nil
	})
	a.Update(MessagesLoadedMsg{ChannelID: "C1", Messages: []messages.MessageItem{{TS: "1700000900.000000", Text: "newer"}}})
	openDialog(t, a, 'p')
	pressKey(t, a, keyCode(tea.KeyEnter))
	if around != "C1/"+pinTS {
		t.Errorf("FetchAround %q, want the pin's history window", around)
	}
}

func TestPinsBookmarks_OpenPinInThread(t *testing.T) {
	a, _, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg { return nil })
	openDialog(t, a, 'p')
	a.Update(keyPress('j'))
	pressKey(t, a, keyCode(tea.KeyEnter))
	if !a.threadVisible || a.threadPanel.ThreadTS() != pinThreadTS {
		t.Fatalf("thread visible %v on %q, want %s", a.threadVisible, a.threadPanel.ThreadTS(), pinThreadTS)
	}
	a.Update(ThreadRepliesLoadedMsg{ThreadTS: pinThreadTS, Replies: []messages.MessageItem{
		{TS: "1700000450.000000", Text: "first"},
		{TS: pinReplyTS, Text: "Root cause: the pool ran dry"},
		{TS: "1700000600.000000", Text: "last"},
	}})
	if sel := a.threadPanel.SelectedReply(); sel == nil || sel.TS != pinReplyTS {
		t.Errorf("selected reply %+v, want the pinned reply", sel)
	}
}

func TestPinsBookmarks_OpenPinInHerdrTab(t *testing.T) {
	a, _, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	var got string
	a.SetHerdrTabOpener(func(url, _ string, _ bool) error {
		got = url
		return nil
	})
	openDialog(t, a, 'p')
	pressKey(t, a, keyPress('O'))
	if got != "https://acme.slack.com/archives/C1/p1700000000000100" {
		t.Errorf("herdr tab opened %q, want the pin's permalink", got)
	}
}

func TestPinsBookmarks_OpenBookmark(t *testing.T) {
	for _, tc := range []struct {
		name      string
		downs     int
		inApp     bool
		browser   string
		toChannel string
	}{
		{name: "web", downs: 0, browser: "https://wiki.acme.com/runbook"},
		{name: "message in slk", downs: 1, inApp: true, toChannel: "C2"},
		{name: "channel in slk", downs: 2, inApp: true, toChannel: "C2"},
		{name: "canvas", downs: 3, browser: "https://acme.slack.com/docs/T1/F07QK2M9HD1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, f, opened := pinsApp(t)
			openChannel(t, a, "C1", "checkout")
			if got := a.bookmarkRows(f.bookmarks["C1"])[tc.downs].InApp; got != tc.inApp {
				t.Errorf("[slk] badge %v, want %v", got, tc.inApp)
			}
			openDialog(t, a, 'B')
			for range tc.downs {
				a.Update(keyPress('j'))
			}
			selected := ""
			for _, m := range pressKey(t, a, keyCode(tea.KeyEnter)) {
				if cs, ok := m.(ChannelSelectedMsg); ok {
					selected = cs.ID
				}
			}
			if *opened != tc.browser {
				t.Errorf("browser opened %q, want %q", *opened, tc.browser)
			}
			if selected != tc.toChannel {
				t.Errorf("switched to channel %q, want %q", selected, tc.toChannel)
			}
		})
	}
}

// o on a link to a channel of this workspace in a message opens the
// channel in slk, as Slack does.
func TestOpenLink_ChannelLinkOpensInSlk(t *testing.T) {
	a, _, opened := pinsApp(t)
	_, cmd := a.Update(OpenLinkMsg{URL: "https://acme.slack.com/archives/C2"})
	var selected string
	for _, m := range drainCmd(cmd) {
		if cs, ok := m.(ChannelSelectedMsg); ok {
			selected = cs.ID
		}
	}
	if selected != "C2" || *opened != "" {
		t.Errorf("switched to %q, browser %q; want C2 in slk", selected, *opened)
	}
	a.Update(OpenLinkMsg{URL: "https://other.slack.com/archives/C2"})
	if *opened != "https://other.slack.com/archives/C2" {
		t.Errorf("another workspace's channel link: browser %q", *opened)
	}
}

func TestPinsBookmarks_Clicks(t *testing.T) {
	a, _, opened := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	openDialog(t, a, 'p')
	w, h := a.pinsBookmarks.model.BoxSize(a.width, a.height)
	x, y := (a.width-w)/2, (a.height-h)/2
	row := -1
	for i, l := range strings.Split(stripANSI(a.pinsBookmarks.model.View(a.width, a.height)), "\n") {
		if strings.Contains(l, "Runbook") {
			row = i
		}
	}
	_, cmd := a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: x + 4, Y: y + row})
	for _, m := range drainBatch(cmd) {
		a.Update(m)
	}
	if *opened != "https://wiki.acme.com/runbook" || a.mode != ModeNormal {
		t.Errorf("a click on Runbook: browser %q, mode %v", *opened, a.mode)
	}

	// Outside the box closes it, a filter typed or not.
	a.Update(keyPress('p'))
	a.Update(keyPress('/'))
	a.Update(keyPress('r'))
	a.Update(tea.MouseClickMsg{Button: tea.MouseLeft, X: 0, Y: 0})
	if a.mode != ModeNormal || a.pinsBookmarks.model.IsVisible() {
		t.Errorf("a click outside: mode %v, visible %v", a.mode, a.pinsBookmarks.model.IsVisible())
	}
}

// Focusing a split window draws the count line of its channel as it
// stands now, counts that came while another window had focus included.
func TestPinsBookmarks_CountLineFollowsWindowFocus(t *testing.T) {
	a, _, _ := pinsApp(t)
	openChannel(t, a, "C1", "checkout")
	first := a.focusedWin
	_ = a.splitWindow(wintree.SplitSideBySide)
	openChannel(t, a, "C2", "payments")
	a.Update(PinCountMsg{TeamID: "T1", ChannelID: "C1", Count: 5})
	for _, msg := range drainCmd(a.focusWindow(first)) {
		a.Update(msg)
	}
	if s := screenText(a); !strings.Contains(s, "📌 5 pins") {
		t.Errorf("focusing C1's window left its count line stale:\n%s", s)
	}
}
