package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

const clickLinkURL = "https://example.com/docs"

// clickLinkApp shows a message with a link labeled "clickme" in the
// messages pane. Opened URLs and thread fetches are recorded instead of
// performed.
func clickLinkApp(t *testing.T) (a *App, opened *[]string, fetched *[]string) {
	t.Helper()
	opened, fetched = new([]string), new([]string)
	a = newHarnessApp(t,
		withHarnessSize(200, 40),
		withHarnessMessages(messages.MessageItem{
			TS: "1.0", UserID: "U1", UserName: "alice", Timestamp: "1:00 PM",
			Text: "see <" + clickLinkURL + "|clickme> for more",
		}),
		withApp(func(a *App) {
			a.activeChannelID = "C1"
			a.browserOpener = func(url string) tea.Cmd {
				*opened = append(*opened, url)
				return nil
			}
			a.setThreadFetcherForTest(func(_ ids.ChannelID, ts ids.ThreadTS) core.Msg {
				*fetched = append(*fetched, string(ts))
				return nil
			})
		}),
	)
	return a, opened, fetched
}

// screenPos finds text on the rendered frame and returns the terminal
// cell of its first character.
func screenPos(t *testing.T, a *App, text string) (x, y int) {
	t.Helper()
	frame := ansi.Strip(a.View().Content)
	for y, row := range strings.Split(frame, "\n") {
		if i := strings.Index(row, text); i >= 0 {
			return ansi.StringWidth(row[:i]), y
		}
	}
	t.Fatalf("%q not on screen:\n%s", text, frame)
	return 0, 0
}

// click presses and releases at (x, y), drawing a frame between the two
// events as the runtime does, and runs every command the release
// returns through Update.
func click(a *App, x, y int) {
	_, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	_ = a.View()
	release(a, x, y)
}

// drag presses at (x, y), moves to (x+dx, y) and releases there.
func drag(a *App, x, y, dx int) {
	_, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	_ = a.View()
	_, _ = a.Update(tea.MouseMotionMsg{X: x + dx, Y: y, Button: tea.MouseLeft})
	release(a, x+dx, y)
}

// release also runs the commands the release's own messages return, one
// level down: the opened link's, such as the profile card's fetch.
func release(a *App, x, y int) {
	_, cmd := a.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	for _, msg := range drainCmds(cmd) {
		_, next := a.Update(msg)
		for _, msg := range drainCmds(next) {
			_, _ = a.Update(msg)
		}
	}
}

func TestClickOnLinkInMessagesPaneOpensIt(t *testing.T) {
	a, opened, fetched := clickLinkApp(t)
	x, y := screenPos(t, a, "clickme")
	for _, cell := range []int{0, 6} { // first and last cell of the label
		*opened = nil
		click(a, x+cell, y)
		if len(*opened) != 1 || (*opened)[0] != clickLinkURL {
			t.Errorf("click at label cell %d opened %v, want [%s]", cell, *opened, clickLinkURL)
		}
	}
	if len(*fetched) != 0 || a.threadVisible {
		t.Errorf("a click on a link opened the thread (fetched %v)", *fetched)
	}
	if a.messagepane.HasSelection() {
		t.Error("a click on a link left a selection")
	}
}

func TestClickBesideLinkInMessagesPaneOpensThreadAsBefore(t *testing.T) {
	a, opened, fetched := clickLinkApp(t)
	x, y := screenPos(t, a, "clickme")
	click(a, x-2, y) // the space before "clickme" is "see "'s
	if len(*opened) != 0 {
		t.Errorf("a click beside the link opened %v", *opened)
	}
	if len(*fetched) != 1 || (*fetched)[0] != "1.0" || !a.threadVisible {
		t.Errorf("a click beside the link: fetched %v, threadVisible %v; want the thread of 1.0 open", *fetched, a.threadVisible)
	}
}

func TestDragFromLinkInMessagesPaneDoesNotOpenIt(t *testing.T) {
	a, opened, _ := clickLinkApp(t)
	x, y := screenPos(t, a, "clickme")
	drag(a, x, y, 5)
	if len(*opened) != 0 {
		t.Errorf("a drag that started on a link opened %v", *opened)
	}
}

// A press on a message cut off at the pane's top selects it, and the
// next frame scrolls it fully into view, moving the rows under the
// pointer before the release. The link opened is the one pressed.
func TestClickOnLinkOfACutOffMessageOpensThePressedLink(t *testing.T) {
	filler := make([]messages.MessageItem, 20)
	for i := range filler {
		filler[i] = messages.MessageItem{TS: fmt.Sprintf("%d.0", i+2), UserID: "U2", UserName: "bob", Timestamp: "1:01 PM", Text: "filler"}
	}
	a, opened, _ := clickLinkApp(t)
	a.messagepane.SetMessages(append([]messages.MessageItem{{
		TS: "1.0", UserID: "U1", UserName: "alice", Timestamp: "1:00 PM",
		Text: "first line\nsecond line\nthird <" + clickLinkURL + "|cutlink> line",
	}}, filler...))
	// Scroll until the message's first line is above the pane but its
	// link row is still in view.
	cut := false
	for range 40 {
		frame := ansi.Strip(a.View().Content)
		if cut = strings.Contains(frame, "cutlink") && !strings.Contains(frame, "first line"); cut {
			break
		}
		a.messagepane.ScrollUp(1)
	}
	if !cut {
		t.Fatal("setup: could not cut the message off at the pane's top")
	}
	x, y := screenPos(t, a, "cutlink")
	click(a, x, y)
	if len(*opened) != 1 || (*opened)[0] != clickLinkURL {
		t.Errorf("click on the cut-off message's link opened %v, want [%s]", *opened, clickLinkURL)
	}
}

func TestClickOnLinkInThreadPanelOpensIt(t *testing.T) {
	a, opened, _ := clickLinkApp(t)
	a.threadVisible = true
	a.threadPanel.SetThread(
		messages.MessageItem{TS: "5.0", UserID: "U1", UserName: "alice", Timestamp: "1:05 PM", Text: "parent <" + clickLinkURL + "|parentlink>"},
		[]messages.MessageItem{{TS: "5.1", ThreadTS: "5.0", UserID: "U2", UserName: "bob", Timestamp: "1:06 PM",
			Text: "read <" + clickLinkURL + "|replylink> first"}},
		"C1", "5.0")
	x, y := screenPos(t, a, "replylink")
	click(a, x, y)
	if len(*opened) != 1 || (*opened)[0] != clickLinkURL {
		t.Errorf("click on the reply's link opened %v, want [%s]", *opened, clickLinkURL)
	}
	if a.threadPanel.HasSelection() {
		t.Error("a click on a link left a selection in the thread panel")
	}

	*opened = nil
	x, y = screenPos(t, a, "parentlink")
	click(a, x, y)
	if len(*opened) != 1 || (*opened)[0] != clickLinkURL {
		t.Errorf("click on the parent's link opened %v, want [%s]", *opened, clickLinkURL)
	}

	*opened = nil
	x, y = screenPos(t, a, "read ")
	click(a, x, y)
	if len(*opened) != 0 {
		t.Errorf("a click beside the reply's link opened %v", *opened)
	}
}
