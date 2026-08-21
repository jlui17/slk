package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

func pressO(app *App) tea.Cmd {
	return app.handleNormalMode(tea.KeyPressMsg{Code: 'o', Text: "o"})
}

func pressShiftO(app *App) tea.Cmd {
	return app.handleNormalMode(tea.KeyPressMsg{Code: 'O', Text: "O"})
}

func TestOpenLinkKey_NoLinks_Toasts(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", Text: "no links here"}})
	cmd := pressO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	if _, ok := cmd().(ToastMsg); !ok {
		t.Errorf("expected ToastMsg, got %#v", cmd())
	}
}

func TestOpenLinkKey_SingleLink_DispatchesOpenLinkMsg(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "see <https://example.com/docs|docs>"},
	})
	cmd := pressO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://example.com/docs" {
		t.Errorf("URL = %q", msg.URL)
	}
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (no modal for single link)", app.mode)
	}
}

func TestOpenLinkKey_MultipleLinks_OpensPicker(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1|one> and <https://b.example/2>"},
	})
	cmd := pressO(app)
	if cmd != nil {
		t.Errorf("expected nil cmd (modal opens), got %#v", cmd())
	}
	if app.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want ModeLinkPicker", app.mode)
	}
	if !app.linkPicker.IsVisible() {
		t.Fatal("picker not visible")
	}
	items := app.linkPicker.Items()
	if len(items) != 2 || items[0].URL != "https://a.example/1" || items[1].URL != "https://b.example/2" {
		t.Errorf("items = %#v", items)
	}
}

func TestLinkPickerMode_EnterDispatchesOpenLinkMsg(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1> <https://b.example/2>"},
	})
	pressO(app)
	cmd := app.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_ = cmd
	cmd = app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected cmd from enter")
	}
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://b.example/2" {
		t.Errorf("URL = %q", msg.URL)
	}
	if app.mode != ModeNormal {
		t.Errorf("mode = %v after choose", app.mode)
	}
}

func TestLinkPickerMode_EscCloses(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1> <https://b.example/2>"},
	})
	pressO(app)
	cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	if cmd != nil {
		t.Errorf("expected nil cmd, got %#v", cmd())
	}
	if app.mode != ModeNormal || app.linkPicker.IsVisible() {
		t.Errorf("mode=%v visible=%v after esc", app.mode, app.linkPicker.IsVisible())
	}
}

func TestOpenLinkKey_FromThreadPanel(t *testing.T) {
	app := NewApp()
	parent := messages.MessageItem{TS: "1.0", Text: "parent"}
	replies := []messages.MessageItem{
		{TS: "1.0", Text: "parent"},
		{TS: "2.0", Text: "see <https://example.com/from-thread>"},
	}
	app.threadPanel.SetThread(parent, replies, "C1", "1.0")
	app.threadVisible = true
	app.focusedPanel = PanelThread
	for i := 0; i < len(replies); i++ {
		if sel := app.threadPanel.SelectedReply(); sel != nil && sel.TS == "2.0" {
			break
		}
		app.threadPanel.MoveDown()
	}
	cmd := pressO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(OpenLinkMsg)
	if !ok || msg.URL != "https://example.com/from-thread" {
		t.Errorf("got %#v", cmd())
	}
}

func TestOpenLinkTabKey_SingleLink_SetsInHerdrTab(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "see <https://example.com/docs|docs>"},
	})
	cmd := pressShiftO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://example.com/docs" || !msg.InHerdrTab {
		t.Errorf("msg = %+v, want InHerdrTab set", msg)
	}
}

func TestOpenLinkTabKey_Picker_EnterSetsInHerdrTab(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1> <https://b.example/2>"},
	})
	pressShiftO(app)
	if app.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want ModeLinkPicker", app.mode)
	}
	cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://a.example/1" || !msg.InHerdrTab {
		t.Errorf("msg = %+v, want InHerdrTab set", msg)
	}
}

// With an opener installed, O offers only slk-openable links: a mixed
// message direct-opens its single permalink, no picker.
func TestOpenLinkTabKey_FiltersToSlackLinks(t *testing.T) {
	app, _ := linkTestApp(t)
	app.SetHerdrTabOpener(func(url, label string) error { return nil })
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://github.com/foo> <https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139> <https://example.com/x>"},
	})
	cmd := pressShiftO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139" || !msg.InHerdrTab {
		t.Errorf("msg = %+v", msg)
	}
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal (single navigable link, no picker)", app.mode)
	}
}

func TestOpenLinkTabKey_NoSlackLinks_Toasts(t *testing.T) {
	app, _ := linkTestApp(t)
	app.SetHerdrTabOpener(func(url, label string) error { return nil })
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://github.com/foo> <https://example.com/x>"},
	})
	cmd := pressShiftO(app)
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	toast, ok := cmd().(ToastMsg)
	if !ok || toast.Text != "No slk-openable links in message" {
		t.Errorf("got %#v, want the no-openable-links toast", cmd())
	}
}

// Without an opener O degrades to o wholesale: no filtering, the
// picker shows every link.
func TestOpenLinkTabKey_NoOpener_KeepsAllLinks(t *testing.T) {
	app, _ := linkTestApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://github.com/foo> <https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139>"},
	})
	pressShiftO(app)
	if app.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want ModeLinkPicker", app.mode)
	}
	if items := app.linkPicker.Items(); len(items) != 2 {
		t.Errorf("items = %#v, want both links", items)
	}
}

// The picker's title only promises a herdr tab when an opener is
// installed; outside herdr, O's picker navigates like o's and must
// say so.
func TestOpenLinkTabKey_Picker_TitleReflectsOpener(t *testing.T) {
	app, _ := linkTestApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139> <https://myteam.slack.com/archives/C054JFCBN69/p1779284734000000>"},
	})
	pressShiftO(app)
	if got := app.linkPicker.Title(); got != "Open link" {
		t.Errorf("title without opener = %q, want \"Open link\"", got)
	}
	app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	app.SetHerdrTabOpener(func(url, label string) error { return nil })
	pressShiftO(app)
	if got := app.linkPicker.Title(); got != "Open link in herdr tab" {
		t.Errorf("title with opener = %q, want \"Open link in herdr tab\"", got)
	}
}

// TestOpenLinkTabKey_FlagClearedOnClose guards the shared picker
// state: escaping an O-opened picker must not leak InHerdrTab into a
// later o-opened one.
func TestOpenLinkTabKey_FlagClearedOnClose(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1> <https://b.example/2>"},
	})
	pressShiftO(app)
	app.handleKey(tea.KeyPressMsg{Code: tea.KeyEscape})
	pressO(app)
	cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.InHerdrTab {
		t.Error("InHerdrTab leaked from the escaped O picker")
	}
}

// TestLinkPickerMode_LinkKindUnaffected guards the shared picker:
// opening it for links must still dispatch OpenLinkMsg, not
// DownloadFileMsg.
func TestLinkPickerMode_LinkKindUnaffected(t *testing.T) {
	app := NewApp()
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://a.example/1> <https://b.example/2>"},
	})
	pressO(app)
	cmd := app.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmd().(OpenLinkMsg)
	if !ok {
		t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
	}
	if msg.URL != "https://a.example/1" {
		t.Errorf("URL = %q", msg.URL)
	}
}
