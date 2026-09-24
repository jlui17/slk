package ui

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

var threePermalinks = []string{
	"https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139",
	"https://myteam.slack.com/archives/C054JFCBN69/p1779284734000000",
	"https://myteam.slack.com/archives/C054JFCBN69/p1779284735000000",
}

// markPickerApp returns an app whose selected message carries
// threePermalinks, with or without a herdr tab opener installed.
func markPickerApp(t *testing.T, withOpener bool) *App {
	t.Helper()
	app, _ := linkTestApp(t)
	if withOpener {
		app.SetHerdrTabOpener(func(url, label string, focus bool) error { return nil })
	}
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<" + threePermalinks[0] + "> <" + threePermalinks[1] + "> <" + threePermalinks[2] + ">"},
	})
	return app
}

func pressPickerKeys(app *App, keys ...tea.KeyPressMsg) tea.Cmd {
	var cmd tea.Cmd
	for _, k := range keys {
		cmd = app.handleKey(k)
	}
	return cmd
}

var (
	keySpace = tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	keyA     = tea.KeyPressMsg{Code: 'a', Text: "a"}
	keyJ     = tea.KeyPressMsg{Code: 'j', Text: "j"}
	keyK     = tea.KeyPressMsg{Code: 'k', Text: "k"}
	keyEnter = tea.KeyPressMsg{Code: tea.KeyEnter}
)

func TestLinkPickerMarks_EnterOpensMarkedInListOrder(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
		want []string
	}{
		// Marked bottom-up: the batch still comes out in list order.
		{"space marks the cursor row", []tea.KeyPressMsg{keyJ, keyJ, keySpace, keyK, keyK, keySpace, keyEnter}, []string{threePermalinks[0], threePermalinks[2]}},
		{"a marks all", []tea.KeyPressMsg{keyA, keyEnter}, threePermalinks},
		{"a over a partial mark marks all", []tea.KeyPressMsg{keySpace, keyA, keyEnter}, threePermalinks},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := markPickerApp(t, true)
			pressShiftO(app)
			cmd := pressPickerKeys(app, tt.keys...)
			if cmd == nil {
				t.Fatal("expected cmd")
			}
			msg, ok := cmd().(OpenLinksInHerdrTabsMsg)
			if !ok {
				t.Fatalf("expected OpenLinksInHerdrTabsMsg, got %#v", cmd())
			}
			if !slices.Equal(msg.URLs, tt.want) {
				t.Errorf("URLs = %v, want %v", msg.URLs, tt.want)
			}
			if app.mode != ModeNormal || app.linkPicker.IsVisible() || app.pickerInTab {
				t.Errorf("picker state not cleared: mode=%v visible=%v pickerInTab=%v", app.mode, app.linkPicker.IsVisible(), app.pickerInTab)
			}
		})
	}
}

func TestLinkPickerMarks_EnterWithoutMarks_OpensCursorRowFocused(t *testing.T) {
	tests := []struct {
		name string
		keys []tea.KeyPressMsg
	}{
		{"nothing marked", []tea.KeyPressMsg{keyJ, keyEnter}},
		{"a twice clears all", []tea.KeyPressMsg{keyA, keyA, keyJ, keyEnter}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := markPickerApp(t, true)
			pressShiftO(app)
			cmd := pressPickerKeys(app, tt.keys...)
			if cmd == nil {
				t.Fatal("expected cmd")
			}
			msg, ok := cmd().(OpenLinkMsg)
			if !ok {
				t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
			}
			if msg.URL != threePermalinks[1] || !msg.InHerdrTab {
				t.Errorf("msg = %+v, want the cursor row with InHerdrTab", msg)
			}
		})
	}
}

// Marking exists only where enter can open a batch of herdr tabs: o's
// picker and an opener-less O's picker keep today's single-choice keys.
func TestLinkPickerMarks_OffOutsideHerdrTabPicker(t *testing.T) {
	tests := []struct {
		name       string
		withOpener bool
		open       func(*App) tea.Cmd
		wantInTab  bool
	}{
		{"o with opener", true, pressO, false},
		{"O without opener", false, pressShiftO, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := markPickerApp(t, tt.withOpener)
			tt.open(app)
			if app.linkPicker.MultiSelect() {
				t.Fatal("MultiSelect on")
			}
			cmd := pressPickerKeys(app, keySpace, keyA, keyEnter)
			if cmd == nil {
				t.Fatal("expected cmd")
			}
			msg, ok := cmd().(OpenLinkMsg)
			if !ok {
				t.Fatalf("expected OpenLinkMsg, got %#v", cmd())
			}
			if msg.URL != threePermalinks[0] || msg.InHerdrTab != tt.wantInTab {
				t.Errorf("msg = %+v", msg)
			}
		})
	}
}

// Escaping a marked picker must not leak marks or multi-select into
// the next picker.
func TestLinkPickerMarks_EscClears(t *testing.T) {
	app := markPickerApp(t, true)
	pressShiftO(app)
	pressPickerKeys(app, keySpace, tea.KeyPressMsg{Code: tea.KeyEscape})
	if app.mode != ModeNormal || app.pickerInTab {
		t.Errorf("mode=%v pickerInTab=%v after esc", app.mode, app.pickerInTab)
	}
	pressO(app)
	if app.linkPicker.MultiSelect() || len(app.linkPicker.Marked()) != 0 {
		t.Errorf("o picker after esc: MultiSelect=%v Marked=%v", app.linkPicker.MultiSelect(), app.linkPicker.Marked())
	}
}
