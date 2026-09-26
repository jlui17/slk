package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/linkpicker"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/wintree"
)

const twoCodeBlocks = "two blocks\n```\nfirst()\n```\nand\n```<go>\n\n\tsecond &lt;2&gt;\n\treturn\n```"

const twoLinks = "see <https://github.com/foo/bar|the PR> and <https://example.com/x?a=1&amp;b=2>"

const linkThenCodeBlock = "see <https://github.com/foo/bar|the PR> then run\n```<sh>\nmake test\n```"

func pressC(app *App) tea.Cmd {
	return app.handleNormalMode(tea.KeyPressMsg{Code: 'c', Text: "c"})
}

func captureClipboard(app *App) *string {
	var copied string
	app.SetClipboardWriter(func(text string) tea.Cmd {
		copied = text
		return nil
	})
	return &copied
}

func assertCopied(t *testing.T, cmd tea.Cmd, copied *string, want string) {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a cmd")
	}
	cMsg, found := drainForCopiedMsg(cmd())
	if !found {
		t.Fatal("expected statusbar.CopiedMsg in batch")
	}
	if *copied != want {
		t.Errorf("clipboard = %q, want %q", *copied, want)
	}
	if cMsg.N != len([]rune(want)) {
		t.Errorf("CopiedMsg.N = %d, want %d", cMsg.N, len([]rune(want)))
	}
}

func assertLinkCopied(t *testing.T, cmd tea.Cmd, copied *string, want string) {
	t.Helper()
	if *copied != want {
		t.Errorf("clipboard = %q, want %q", *copied, want)
	}
	for _, msg := range drainCmd(cmd) {
		if toast, ok := msg.(ToastMsg); ok && toast.Text == "Copied link" {
			return
		}
	}
	t.Error(`expected the "Copied link" toast`)
}

func assertCopyPickerOpen(t *testing.T, app *App, rows int) []linkpicker.Item {
	t.Helper()
	if app.mode != ModeLinkPicker || !app.linkPicker.IsVisible() || app.pickerKind != "copy" {
		t.Fatalf("mode=%v visible=%v kind=%q, want the copy picker", app.mode, app.linkPicker.IsVisible(), app.pickerKind)
	}
	if app.linkPicker.Title() != "Copy from message" || app.linkPicker.MultiSelect() {
		t.Errorf("title=%q multiSelect=%v", app.linkPicker.Title(), app.linkPicker.MultiSelect())
	}
	items := app.linkPicker.Items()
	if len(items) != rows {
		t.Fatalf("items = %#v, want %d", items, rows)
	}
	return items
}

func assertCopyPickerClosed(t *testing.T, app *App) {
	t.Helper()
	if app.mode != ModeNormal || app.linkPicker.IsVisible() || app.pickerKind != "" || app.pickerCopyables != nil {
		t.Errorf("mode=%v visible=%v kind=%q copyables=%v, want the picker closed and cleared", app.mode, app.linkPicker.IsVisible(), app.pickerKind, app.pickerCopyables)
	}
}

func TestCopyFromMessageKey_NothingToCopy_Toasts(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "only `inline` code"}})
	cmd := pressC(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	toast, ok := cmd().(ToastMsg)
	if !ok || toast.Text != "Nothing to copy in message" {
		t.Errorf("got %#v, want the nothing-to-copy toast", cmd())
	}
	if *copied != "" {
		t.Errorf("clipboard = %q, want it untouched", *copied)
	}
}

func TestCopyFromMessageKey_OneBlock_CopiesIt(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "run\n```\n\tif a &lt; b {}\n```\nthen `done`"}})
	assertCopied(t, pressC(app), copied, "\tif a < b {}")
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (no picker for one block)", app.mode)
	}
}

// The clipboard gets the URL alone: no label, no angle brackets.
func TestCopyFromMessageKey_OneLink_CopiesItsURL(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "see <https://github.com/foo/bar|the PR>"}})
	assertLinkCopied(t, pressC(app), copied, "https://github.com/foo/bar")
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (no picker for one link)", app.mode)
	}
}

// Slack sends & inside a URL as &amp;; the copied URL has to work
// where it is pasted.
func TestCopyFromMessageKey_DecodesAmpersandsInALink(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "<https://example.com/x?a=1&amp;b=2>"}})
	assertLinkCopied(t, pressC(app), copied, "https://example.com/x?a=1&b=2")
}

// The block carries the link, so there is one thing to copy and no picker.
func TestCopyFromMessageKey_LinkInsideABlock_CopiesTheBlock(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "run\n```\ncurl <https://example.com/install.sh>\n```"}})
	assertCopied(t, pressC(app), copied, "curl <https://example.com/install.sh>")
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (no picker for one item)", app.mode)
	}
}

func TestCopyFromMessageKey_BlockAndLink_PickerListsBothInMessageOrder(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: linkThenCodeBlock}})
	if msgs := drainCmd(pressC(app)); len(msgs) != 0 {
		t.Errorf("expected no msgs (picker opens), got %#v", msgs)
	}
	items := assertCopyPickerOpen(t, app, 2)
	if items[0].Label != "the PR" || items[0].URL != "https://github.com/foo/bar" {
		t.Errorf("row 0 = %#v, want the link", items[0])
	}
	if items[1].Label != "sh" || items[1].Display != "make test" || items[1].Detail != "1 line" {
		t.Errorf("row 1 = %#v, want the code block", items[1])
	}
	if *copied != "" {
		t.Errorf("clipboard = %q before a row was chosen", *copied)
	}
}

func TestCopyFromMessagePicker_EnterOnTheCodeRowCopiesTheCode(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: linkThenCodeBlock}})
	pressC(app)
	app.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assertCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "make test")
	assertCopyPickerClosed(t, app)
}

func TestCopyFromMessagePicker_EnterOnTheLinkRowCopiesTheURL(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: linkThenCodeBlock}})
	pressC(app)
	assertLinkCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "https://github.com/foo/bar")
	assertCopyPickerClosed(t, app)
}

func TestCopyFromMessageKey_ManyBlocks_PickerCopiesTheChosenOne(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoCodeBlocks}})
	if msgs := drainCmd(pressC(app)); len(msgs) != 0 {
		t.Errorf("expected no msgs (picker opens), got %#v", msgs)
	}
	items := assertCopyPickerOpen(t, app, 2)
	if items[0].Label != "code" || items[0].Display != "first()" || items[0].Detail != "1 line" {
		t.Errorf("row 0 = %#v", items[0])
	}
	if items[1].Label != "go" || items[1].Display != "second <2>" || items[1].Detail != "2 lines" {
		t.Errorf("row 1 = %#v", items[1])
	}

	app.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assertCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "\tsecond <2>\n\treturn")
	assertCopyPickerClosed(t, app)
}

func TestCopyFromMessageKey_ManyLinks_PickerCopiesTheChosenOne(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoLinks}})
	if msgs := drainCmd(pressC(app)); len(msgs) != 0 {
		t.Errorf("expected no msgs (picker opens), got %#v", msgs)
	}
	items := assertCopyPickerOpen(t, app, 2)
	if items[0].Label != "the PR" || items[0].URL != "https://github.com/foo/bar" {
		t.Errorf("row 0 = %#v", items[0])
	}
	if *copied != "" {
		t.Errorf("clipboard = %q before a row was chosen", *copied)
	}

	app.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assertLinkCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "https://example.com/x?a=1&b=2")
	assertCopyPickerClosed(t, app)
}

// Space and `a` mark rows only in O's herdr-tab picker: here Enter after
// them still copies the cursor row alone.
func TestCopyFromMessagePicker_MultiSelectOff(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoLinks}})
	pressC(app)
	app.handleKey(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	app.handleKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	if app.linkPicker.MultiSelect() {
		t.Error("the copy picker must not mark rows")
	}
	assertLinkCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "https://github.com/foo/bar")
}

// Permalink rows read as they do in the `o` picker: decoded fallback
// display with the URL as detail, then the fetched message preview.
func TestCopyFromMessagePicker_PermalinkRowsMatchTheOpenPicker(t *testing.T) {
	app := linkPreviewTestApp(t)
	copied := captureClipboard(app)
	pressO(app)
	want := append([]linkpicker.Item(nil), app.linkPicker.Items()...)
	app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})

	msgs := drainCmd(pressC(app))
	got := app.linkPicker.Items()
	if len(got) != len(want) {
		t.Fatalf("rows = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %#v, want %#v", i, got[i], want[i])
		}
	}
	if len(msgs) != 1 {
		t.Fatalf("preview msgs = %#v, want 1 (in-app row only)", msgs)
	}
	app.Update(msgs[0])
	if display := app.linkPicker.Items()[0].Display; display != "#general · matt: deploy is done see #general for details" {
		t.Errorf("Display = %q, want the fetched preview", display)
	}
	assertLinkCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139")
}

func TestCopyFromMessagePicker_EscCopiesNothing(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoCodeBlocks}})
	pressC(app)
	if cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
		t.Errorf("expected nil cmd, got %#v", cmd())
	}
	assertCopyPickerClosed(t, app)
	if *copied != "" {
		t.Errorf("clipboard = %q after esc", *copied)
	}
}

func TestCopyFromMessageKey_FromThreadPane(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	parent := messages.MessageItem{TS: "1.0", Text: "```\nfrom the parent\n```"}
	replies := []messages.MessageItem{
		{TS: "2.0", Text: "<https://example.com/reply|a reply link>"},
		{TS: "3.0", Text: "```\nfrom the reply\n```"},
		{TS: "4.0", Text: twoCodeBlocks},
	}
	app.threadPanel.SetThread(parent, replies, "C1", "1.0")
	app.threadVisible = true
	app.focusedPanel = PanelThread

	// The cursor opens on the newest reply.
	pressC(app)
	assertCopyPickerOpen(t, app, 2)
	assertCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "first()")

	app.threadPanel.MoveUp()
	assertCopied(t, pressC(app), copied, "from the reply")

	app.threadPanel.MoveUp()
	assertLinkCopied(t, pressC(app), copied, "https://example.com/reply")

	app.threadPanel.MoveUp()
	assertCopied(t, pressC(app), copied, "from the parent")
}

func TestCopyFromMessageKey_NothingSelectedNoop(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	if cmd := pressC(app); cmd != nil {
		t.Errorf("expected nil cmd with nothing selected, got %#v", cmd())
	}
}

// c after ctrl+w still closes the window: the chord is read before the
// normal-mode bindings.
func TestCopyFromMessageKey_WindowChordStillCloses(t *testing.T) {
	app := newTestApp(t, withSize(200, 50), withMessages(messages.MessageItem{TS: "1.0", Text: "```\ncode\n```"}), withWindowSplit(wintree.SplitSideBySide))
	copied := captureClipboard(app)
	if app.wins.Len() != 2 {
		t.Fatalf("precondition: %d windows, want 2", app.wins.Len())
	}
	app.handleNormalMode(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	pressC(app)
	if app.wins.Len() != 1 {
		t.Errorf("windows = %d after ctrl+w c, want 1", app.wins.Len())
	}
	if *copied != "" {
		t.Errorf("ctrl+w c copied %q", *copied)
	}
}

func TestDefaultKeyMap_CopyFromMessage(t *testing.T) {
	km := DefaultKeyMap()
	if keys := km.CopyFromMessage.Keys(); len(keys) != 1 || keys[0] != "c" {
		t.Errorf("CopyFromMessage keys = %v, want [c]", keys)
	}
	if km.CopyFromMessage.Help().Desc != "copy code block or link in message" {
		t.Errorf("CopyFromMessage help = %q", km.CopyFromMessage.Help().Desc)
	}
}

// copyLabelCells finds the copy labels on the drawn screen: the terminal
// (x, y) of each label's first cell, top to bottom.
func copyLabelCells(a *App) [][2]int {
	var cells [][2]int
	for y, row := range strings.Split(ansi.Strip(a.View().Content), "\n") {
		if before, _, found := strings.Cut(row, " copy ─╮"); found {
			cells = append(cells, [2]int{ansi.StringWidth(before), y})
		}
	}
	return cells
}

func TestApp_ClickOnCopyLabelCopiesThatBlock(t *testing.T) {
	a := newHarnessApp(t, withHarnessMessages(
		messages.MessageItem{TS: "1.0", UserName: "alice", Text: "no blocks", Timestamp: "1:00 PM"},
		messages.MessageItem{TS: "2.0", UserName: "bob", Text: twoCodeBlocks, Timestamp: "1:01 PM"},
	))
	a.activeChannelID = "C1"
	copied := captureClipboard(a)
	fetched := false
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		fetched = true
		return ThreadRepliesLoadedMsg{}
	})
	cells := copyLabelCells(a)
	if len(cells) != 2 {
		t.Fatalf("found %d copy labels on screen, want 2", len(cells))
	}
	x, y := cells[1][0]+1, cells[1][1]

	_, cmd := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	assertCopied(t, cmd, copied, "\tsecond <2>\n\treturn")
	_, release := a.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	_ = drainBatch(release)

	if a.messagepane.HasSelection() {
		t.Error("label click must not begin a drag selection")
	}
	if fetched || a.threadVisible {
		t.Error("label click must not open the message's thread")
	}
}

// One row below the label is the block's body: the press selects text
// and the release opens the thread, as before.
func TestApp_ClickBesideCopyLabelBehavesAsBefore(t *testing.T) {
	a := newHarnessApp(t, withHarnessMessages(
		messages.MessageItem{TS: "2.0", UserName: "bob", Text: twoCodeBlocks, Timestamp: "1:01 PM"},
	))
	a.activeChannelID = "C1"
	copied := captureClipboard(a)
	fetched := false
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		fetched = true
		return ThreadRepliesLoadedMsg{}
	})
	cells := copyLabelCells(a)
	if len(cells) != 2 {
		t.Fatalf("found %d copy labels on screen, want 2", len(cells))
	}
	x, y := cells[0][0]+1, cells[0][1]+1

	_, _ = a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	if !a.messagepane.HasSelection() {
		t.Error("press on the block's body should anchor a selection")
	}
	_, release := a.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
	_ = drainBatch(release)
	if !fetched {
		t.Error("plain click on the block's body should open the thread")
	}
	if *copied != "" {
		t.Errorf("clipboard = %q, want it untouched", *copied)
	}
}

func TestApp_ClickOnCopyLabelInThreadPane(t *testing.T) {
	a := newHarnessApp(t, withHarnessSize(160, 40), withApp(func(a *App) {
		a.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", UserName: "alice", Text: "parent", Timestamp: "1:00 PM"}})
		a.threadPanel.SetThread(
			messages.MessageItem{TS: "1.0", UserName: "alice", Text: "```\nfrom the parent\n```", Timestamp: "1:00 PM"},
			[]messages.MessageItem{{TS: "2.0", UserName: "bob", Text: twoCodeBlocks, Timestamp: "1:01 PM"}},
			"C1", "1.0")
		a.threadVisible = true
	}))
	copied := captureClipboard(a)
	cells := copyLabelCells(a)
	if len(cells) != 3 {
		t.Fatalf("found %d copy labels on screen, want 3", len(cells))
	}
	for i, want := range []string{"from the parent", "first()", "\tsecond <2>\n\treturn"} {
		x, y := cells[i][0]+1, cells[i][1]
		_, cmd := a.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
		assertCopied(t, cmd, copied, want)
		_, _ = a.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
		if a.threadPanel.HasSelection() {
			t.Errorf("label %d: click must not begin a drag selection", i)
		}
	}
}
