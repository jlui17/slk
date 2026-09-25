package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/wintree"
)

const twoCodeBlocks = "two blocks\n```\nfirst()\n```\nand\n```<go>\n\n\tsecond &lt;2&gt;\n\treturn\n```"

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

func TestCopyCodeBlockKey_NoBlock_Toasts(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "only `inline` code"}})
	cmd := pressC(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	toast, ok := cmd().(ToastMsg)
	if !ok || toast.Text != "Message has no code block" {
		t.Errorf("got %#v, want the no-code-block toast", cmd())
	}
	if *copied != "" {
		t.Errorf("clipboard = %q, want it untouched", *copied)
	}
}

func TestCopyCodeBlockKey_OneBlock_CopiesIt(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "run\n```\n\tif a &lt; b {}\n```\nthen `done`"}})
	assertCopied(t, pressC(app), copied, "\tif a < b {}")
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (no picker for one block)", app.mode)
	}
}

func TestCopyCodeBlockKey_ManyBlocks_PickerCopiesTheChosenOne(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoCodeBlocks}})
	if cmd := pressC(app); cmd != nil {
		t.Errorf("expected nil cmd (picker opens), got %#v", cmd())
	}
	if app.mode != ModeLinkPicker || !app.linkPicker.IsVisible() || app.pickerKind != "code" {
		t.Fatalf("mode=%v visible=%v kind=%q, want the code picker", app.mode, app.linkPicker.IsVisible(), app.pickerKind)
	}
	if app.linkPicker.Title() != "Copy code block" || app.linkPicker.MultiSelect() {
		t.Errorf("title=%q multiSelect=%v", app.linkPicker.Title(), app.linkPicker.MultiSelect())
	}
	items := app.linkPicker.Items()
	if len(items) != 2 {
		t.Fatalf("items = %#v, want 2", items)
	}
	if items[0].Label != "code" || items[0].Display != "first()" || items[0].Detail != "1 line" {
		t.Errorf("row 0 = %#v", items[0])
	}
	if items[1].Label != "go" || items[1].Display != "second <2>" || items[1].Detail != "2 lines" {
		t.Errorf("row 1 = %#v", items[1])
	}

	app.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	assertCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "\tsecond <2>\n\treturn")
	if app.mode != ModeNormal || app.linkPicker.IsVisible() || app.pickerKind != "" || app.pickerCodeBlocks != nil {
		t.Errorf("after enter: mode=%v visible=%v kind=%q blocks=%v", app.mode, app.linkPicker.IsVisible(), app.pickerKind, app.pickerCodeBlocks)
	}
}

func TestCopyCodeBlockPicker_EscCopiesNothing(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: twoCodeBlocks}})
	pressC(app)
	if cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape}); cmd != nil {
		t.Errorf("expected nil cmd, got %#v", cmd())
	}
	if app.mode != ModeNormal || app.pickerKind != "" || app.pickerCodeBlocks != nil || *copied != "" {
		t.Errorf("after esc: mode=%v kind=%q blocks=%v clipboard=%q", app.mode, app.pickerKind, app.pickerCodeBlocks, *copied)
	}
}

func TestCopyCodeBlockKey_FromThreadPane(t *testing.T) {
	app := NewApp()
	copied := captureClipboard(app)
	parent := messages.MessageItem{TS: "1.0", Text: "```\nfrom the parent\n```"}
	replies := []messages.MessageItem{
		{TS: "2.0", Text: "```\nfrom the reply\n```"},
		{TS: "3.0", Text: twoCodeBlocks},
	}
	app.threadPanel.SetThread(parent, replies, "C1", "1.0")
	app.threadVisible = true
	app.focusedPanel = PanelThread

	// The cursor opens on the newest reply.
	pressC(app)
	if app.mode != ModeLinkPicker || app.pickerKind != "code" {
		t.Fatalf("mode=%v kind=%q, want the code picker", app.mode, app.pickerKind)
	}
	assertCopied(t, app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter}), copied, "first()")

	app.threadPanel.MoveUp()
	assertCopied(t, pressC(app), copied, "from the reply")

	app.threadPanel.MoveUp()
	assertCopied(t, pressC(app), copied, "from the parent")
}

func TestCopyCodeBlockKey_NothingSelectedNoop(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	if cmd := pressC(app); cmd != nil {
		t.Errorf("expected nil cmd with nothing selected, got %#v", cmd())
	}
}

// c after ctrl+w still closes the window: the chord is read before the
// normal-mode bindings.
func TestCopyCodeBlockKey_WindowChordStillCloses(t *testing.T) {
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

func TestDefaultKeyMap_CopyCodeBlock(t *testing.T) {
	km := DefaultKeyMap()
	if keys := km.CopyCodeBlock.Keys(); len(keys) != 1 || keys[0] != "c" {
		t.Errorf("CopyCodeBlock keys = %v, want [c]", keys)
	}
	if km.CopyCodeBlock.Help().Desc == "" {
		t.Error("CopyCodeBlock has no help text")
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
