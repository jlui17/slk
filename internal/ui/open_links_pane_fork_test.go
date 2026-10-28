package ui

import (
	"errors"
	"fmt"
	"slices"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

var keyAltO = tea.KeyPressMsg{Code: 'o', Mod: tea.ModAlt}

// pressAltO goes through handleKey, the whole key path, since no other
// alt+ binding shows that path lets an alt key through to normal mode.
func pressAltO(app *App) tea.Cmd {
	return app.handleKey(keyAltO)
}

// paneCall is one call of a fake herdr pane opener.
type paneCall struct {
	url   string
	split string
	focus bool
}

// installPaneOpener installs a recording pane opener, and a tab opener
// that fails the test, so a press that reaches the wrong one shows. The
// pane opener fails the open of failURL; every other open creates pane
// "p<call number>".
func installPaneOpener(t *testing.T, app *App, failURL string) *[]paneCall {
	t.Helper()
	var calls []paneCall
	app.SetHerdrPaneOpener(func(url, split string, focus bool) (string, error) {
		calls = append(calls, paneCall{url, split, focus})
		if url == failURL {
			return "", errors.New("boom")
		}
		return fmt.Sprintf("p%d", len(calls)), nil
	})
	app.SetHerdrTabOpener(func(url, label string, focus bool) error {
		t.Errorf("herdr tab opener called for %q", url)
		return nil
	})
	return &calls
}

// Bubbletea reports Option/Alt+o as "alt+o", the key the binding names.
func TestOpenLinkPaneKey_AltOMatchesBinding(t *testing.T) {
	if got := keyAltO.String(); got != "alt+o" {
		t.Errorf("alt+o press reads %q, want \"alt+o\"", got)
	}
	keys := DefaultKeyMap()
	if !key.Matches(keyAltO, keys.OpenLinkPane) {
		t.Error("alt+o press does not match OpenLinkPane")
	}
	if key.Matches(keyAltO, keys.OpenLink) || key.Matches(keyAltO, keys.OpenLinkTab) {
		t.Error("alt+o press matches o or O")
	}
}

// alt+o on a message with one permalink opens it in a focused herdr pane,
// not a tab, and skips the in-place navigation.
func TestOpenLinkPaneKey_SingleLink_OpensFocusedPane(t *testing.T) {
	app, opened := linkTestApp(t)
	calls := installPaneOpener(t, app, "")
	url := threePermalinks[0]
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "see <" + url + ">"}})

	msgs := drainCmd(pressAltO(app))
	if len(msgs) != 1 {
		t.Fatalf("msgs = %#v, want one OpenLinkMsg", msgs)
	}
	open, ok := msgs[0].(OpenLinkMsg)
	if !ok || open.URL != url || open.InHerdr != HerdrPane {
		t.Fatalf("got %#v, want OpenLinkMsg of %q in a herdr pane", msgs[0], url)
	}
	_, cmd := app.Update(open)
	drainCmd(cmd)
	if want := []paneCall{{url, "", true}}; !slices.Equal(*calls, want) {
		t.Errorf("pane opener calls = %+v, want %+v", *calls, want)
	}
	if app.pendingLinkNav != nil || *opened != "" {
		t.Errorf("open leaked into in-place nav (%+v) or the browser (%q)", app.pendingLinkNav, *opened)
	}
}

// alt+o works on a reply in the thread panel as it does in the messages
// pane.
func TestOpenLinkPaneKey_ThreadReply(t *testing.T) {
	app, _ := linkTestApp(t)
	installPaneOpener(t, app, "")
	parent := messages.MessageItem{TS: "0.5", Text: "parent"}
	reply := messages.MessageItem{TS: "1.0", ThreadTS: "0.5", Text: "<" + threePermalinks[1] + ">"}
	app.threadPanel.SetThread(parent, []messages.MessageItem{parent, reply}, "C1", "0.5")
	app.threadVisible = true
	app.focusedPanel = PanelThread
	for sel := app.threadPanel.SelectedReply(); sel == nil || sel.TS != "1.0"; sel = app.threadPanel.SelectedReply() {
		app.threadPanel.MoveDown()
	}

	open, ok := pressAltO(app)().(OpenLinkMsg)
	if !ok || open.URL != threePermalinks[1] || open.InHerdr != HerdrPane {
		t.Errorf("got %#v, want OpenLinkMsg of the reply's link in a herdr pane", open)
	}
}

// Outside herdr (no pane opener) alt+o degrades as O does: the link
// navigates in place, and a tab opener is not a stand-in for the pane.
func TestOpenLinkPaneKey_NoPaneOpener_RoutesInPlace(t *testing.T) {
	app, opened := linkTestApp(t)
	app.SetHerdrTabOpener(func(url, label string, focus bool) error {
		t.Errorf("herdr tab opener called for %q", url)
		return nil
	})
	app.activeChannelID = "CELSEWHERE"
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://github.com/foo> <" + threePermalinks[0] + ">"},
	})

	pressAltO(app)
	if app.mode != ModeLinkPicker || len(app.linkPicker.Items()) != 2 || app.linkPicker.Title() != "Open link" {
		t.Fatalf("mode=%v items=%d title=%q, want o's picker with both links", app.mode, len(app.linkPicker.Items()), app.linkPicker.Title())
	}
	open, ok := pressPickerKeys(app, keyJ, keyEnter)().(OpenLinkMsg)
	if !ok {
		t.Fatal("enter gave no OpenLinkMsg")
	}
	_, cmd := app.Update(open)
	drainCmd(cmd)
	if app.pendingLinkNav == nil || string(app.pendingLinkNav.MessageTS) != "1779284733.270139" || !app.pendingLinkNav.openEmptyThread {
		t.Errorf("pendingLinkNav = %+v, want O's in-place nav", app.pendingLinkNav)
	}
	if *opened != "" {
		t.Errorf("browser opened %q", *opened)
	}
}

func TestOpenLinkPaneKey_OpenerError_Toasts(t *testing.T) {
	app, _ := linkTestApp(t)
	installPaneOpener(t, app, threePermalinks[0])
	_, cmd := app.Update(OpenLinkMsg{URL: threePermalinks[0], InHerdr: HerdrPane})
	msgs := drainCmd(cmd)
	if len(msgs) != 1 || msgs[0] != (ToastMsg{Text: "Failed to open herdr pane"}) {
		t.Errorf("msgs = %#v, want the failed-pane toast", msgs)
	}
}

// With several permalinks alt+o opens O's picker, titled for a pane:
// enter opens the cursor row in a focused pane.
func TestOpenLinkPaneKey_Picker_EnterOpensCursorRow(t *testing.T) {
	app := markPickerApp(t, false)
	calls := installPaneOpener(t, app, "")

	pressAltO(app)
	if got := app.linkPicker.Title(); got != "Open link in herdr pane" || !app.linkPicker.MultiSelect() {
		t.Fatalf("title=%q multiselect=%v, want the herdr pane picker with marks", got, app.linkPicker.MultiSelect())
	}
	open, ok := pressPickerKeys(app, keyJ, keyEnter)().(OpenLinkMsg)
	if !ok || open.URL != threePermalinks[1] || open.InHerdr != HerdrPane {
		t.Fatalf("got %#v, want the cursor row in a herdr pane", open)
	}
	_, cmd := app.Update(open)
	drainCmd(cmd)
	if want := []paneCall{{threePermalinks[1], "", true}}; !slices.Equal(*calls, want) {
		t.Errorf("pane opener calls = %+v, want %+v", *calls, want)
	}
}

// A failed open in a batch does not advance the split target: the next
// link splits the last pane that did open, or slk's when none has.
func TestOpenLinksInHerdr_Panes_FailureKeepsSplitTarget(t *testing.T) {
	app, _ := linkTestApp(t)
	calls := installPaneOpener(t, app, threePermalinks[1])

	_, cmd := app.Update(OpenLinksInHerdrMsg{URLs: threePermalinks, Target: HerdrPane})
	msgs := drainCmd(cmd)
	want := []paneCall{{threePermalinks[0], "", false}, {threePermalinks[1], "p1", false}, {threePermalinks[2], "p1", false}}
	if !slices.Equal(*calls, want) {
		t.Errorf("pane opener calls = %+v, want %+v", *calls, want)
	}
	if len(msgs) != 1 || msgs[0] != (ToastMsg{Text: "Opened 2 of 3 herdr panes"}) {
		t.Errorf("msgs = %#v, want the 2-of-3 toast", msgs)
	}

	calls = installPaneOpener(t, app, threePermalinks[0])
	_, cmd = app.Update(OpenLinksInHerdrMsg{URLs: threePermalinks[:2], Target: HerdrPane})
	drainCmd(cmd)
	if want := []paneCall{{threePermalinks[0], "", false}, {threePermalinks[1], "", false}}; !slices.Equal(*calls, want) {
		t.Errorf("first open failed: pane opener calls = %+v, want %+v", *calls, want)
	}
}

// Marked rows each open in their own pane, without moving focus off slk:
// the first splits slk's pane and each next one the pane before it, so
// they read left to right in list order. One toast counts them.
func TestOpenLinkPaneKey_Picker_MarksOpenUnfocusedPanes(t *testing.T) {
	app := markPickerApp(t, false)
	calls := installPaneOpener(t, app, "")

	pressAltO(app)
	batch, ok := pressPickerKeys(app, keyA, keyEnter)().(OpenLinksInHerdrMsg)
	if !ok || batch.Target != HerdrPane || !slices.Equal(batch.URLs, threePermalinks) {
		t.Fatalf("got %#v, want every row opened as herdr panes", batch)
	}
	if app.mode != ModeNormal || app.pickerInHerdr != HerdrNone {
		t.Errorf("picker state not cleared: mode=%v pickerInHerdr=%v", app.mode, app.pickerInHerdr)
	}
	_, cmd := app.Update(batch)
	msgs := drainCmd(cmd)
	want := []paneCall{{threePermalinks[0], "", false}, {threePermalinks[1], "p1", false}, {threePermalinks[2], "p2", false}}
	if !slices.Equal(*calls, want) {
		t.Errorf("pane opener calls = %+v, want %+v", *calls, want)
	}
	if len(msgs) != 1 || msgs[0] != (ToastMsg{Text: "Opened 3 herdr panes"}) {
		t.Errorf("msgs = %#v, want the 3-panes toast", msgs)
	}
}
