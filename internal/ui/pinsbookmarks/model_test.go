package pinsbookmarks

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func fixture() *Model {
	m := &Model{}
	m.Open("#checkout", Pins)
	setLists(m, []PinRow{
		{Author: "maya", Day: "Today", Text: "Postmortem draft is in the template"},
		{Author: "sam", Day: "Yesterday", Text: "Root cause: the pool ran dry", InThread: true},
		{Author: "dev", Day: "Oct 24", Text: "Rollback: run deploy rollback"},
		{Author: "PagerDuty", Day: "Oct 21", Text: "latency.pdf", File: true},
	}, []BookmarkRow{
		{Title: "Checkout runbook", Site: "wiki.acme.com/runbook"},
		{Title: "#payments-eng", Site: "acme.slack.com/archives/C2", InApp: true},
		{Title: "Status page", Site: "status.acme.com"},
	})
	return m
}

func setLists(m *Model, pins []PinRow, bookmarks []BookmarkRow) {
	m.SetPins(pins)
	m.SetBookmarks(bookmarks)
}

func screen(m *Model) string { return ansi.Strip(m.View(160, 40)) }

func press(m *Model, keys ...string) Action {
	var act Action
	for _, k := range keys {
		act = m.HandleKey(k)
	}
	return act
}

// at is where the cursor is.
func at(m *Model) string {
	return fmt.Sprintf("%d:%d", m.section, m.selected[m.section])
}

func TestKeys_JAndKRunThroughBothSections(t *testing.T) {
	m := fixture()
	for _, step := range []struct{ key, want string }{
		{"j", "0:1"}, {"j", "0:2"}, {"j", "0:3"},
		{"j", "1:0"}, // the last pin leads on to the first bookmark
		{"down", "1:1"}, {"j", "1:2"}, {"j", "1:2"},
		{"k", "1:1"}, {"up", "1:0"},
		{"k", "0:3"}, // and back
		{"G", "0:3"}, {"g", "0:3"}, {"g", "0:0"},
		{"tab", "1:0"}, {"G", "1:2"}, {"shift+tab", "0:0"}, {"tab", "1:2"},
	} {
		press(m, step.key)
		if got := at(m); got != step.want {
			t.Fatalf("after %q at %s, want %s", step.key, got, step.want)
		}
	}
}

func TestKeys_PAndBSwitchSectionOrClose(t *testing.T) {
	m := fixture()
	press(m, "B")
	if !m.IsVisible() || m.Section() != Bookmarks {
		t.Fatalf("B on pins: visible=%v section=%v, want bookmarks", m.IsVisible(), m.Section())
	}
	press(m, "p")
	if !m.IsVisible() || m.Section() != Pins {
		t.Fatalf("p on bookmarks: visible=%v section=%v, want pins", m.IsVisible(), m.Section())
	}
	press(m, "p")
	if m.IsVisible() {
		t.Error("p on pins should close the dialog")
	}
	m = fixture()
	press(m, "B", "B")
	if m.IsVisible() {
		t.Error("B on bookmarks should close the dialog")
	}
	m = fixture()
	press(m, "q")
	if m.IsVisible() {
		t.Error("q should close the dialog")
	}
}

func TestKeys_OpenTheCursorsItem(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want Kind
	}{{"enter", Open}, {"o", Open}, {"O", OpenTab}, {"alt+o", OpenPane}} {
		m := fixture()
		got := press(m, "j", "j", "j", "j", "j", tc.key)
		if want := (Action{Kind: tc.want, Section: Bookmarks, Index: 1}); got != want {
			t.Errorf("%s: got %+v, want %+v", tc.key, got, want)
		}
		if m.IsVisible() {
			t.Errorf("%s: opening should close the dialog", tc.key)
		}
	}
}

func TestFilter_BothSections(t *testing.T) {
	m := fixture()
	press(m, "/", "r", "o")
	s := screen(m)
	// "ro" is in "Root" and "Rollback", and in no bookmark.
	for _, want := range []string{"▌ ro█", "Pins  2 of 4", "Bookmarks  0 of 3", "No matches", "2 matches", "↑/↓ move  enter open  esc clear"} {
		if !strings.Contains(s, want) {
			t.Errorf("filtered screen lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "Postmortem") || strings.Contains(s, "Status page") {
		t.Errorf("filter shows rows it should hide:\n%s", s)
	}
	press(m, "o", "t")
	if s := screen(m); !strings.Contains(s, "1 match") || !strings.Contains(s, "Pins  1 of 4") {
		t.Errorf("one match:\n%s", s)
	}
	if at(m) != "0:1" {
		t.Errorf("cursor at %s, want on the one match 0:1", at(m))
	}
	if got := press(m, "enter"); got != (Action{Kind: Open, Section: Pins, Index: 1}) {
		t.Errorf("enter while filtering: %+v", got)
	}

	// A filter only bookmarks match takes the cursor to them.
	m = fixture()
	press(m, "/", "a", "c", "m", "e")
	if s := screen(m); !strings.Contains(s, "Pins  0 of 4") || !strings.Contains(s, "Bookmarks  3 of 3") || at(m) != "1:0" {
		t.Errorf("cursor at %s, want 1:0:\n%s", at(m), s)
	}
}

func TestFilter_EscClearsThenCloses(t *testing.T) {
	m := fixture()
	press(m, "/", "s", "t", "a", "t")
	if at(m) != "1:2" {
		t.Fatalf("cursor at %s, want moved to the only match, Status page", at(m))
	}
	press(m, "tab") // back to the list, filter kept
	if s := screen(m); !strings.Contains(s, "▌ stat") || !strings.Contains(s, "1 match") {
		t.Fatalf("tab should keep the filter:\n%s", s)
	}
	press(m, "esc")
	if !m.IsVisible() || strings.Contains(screen(m), "▌ stat") {
		t.Fatal("first esc should clear the filter and keep the dialog")
	}
	press(m, "esc")
	if m.IsVisible() {
		t.Error("second esc should close the dialog")
	}
	m = fixture()
	press(m, "/", "x", "esc")
	if !m.IsVisible() || m.filtering || m.filter != "" {
		t.Error("esc while typing the filter should drop it and keep the dialog")
	}
}

func TestView_Rows(t *testing.T) {
	s := screen(fixture())
	for _, want := range []string{
		"Pins and bookmarks · #checkout",
		"Pins  4", "Bookmarks  3",
		"maya", "Today", "Postmortem draft is in the template",
		"in thread",
		"[File] latency.pdf",
		"Checkout runbook  wiki.acme.com/runbook",
		"[slk]",
		"tab pins/bookmarks  j/k move  enter open  / filter  esc close  [slk] opens in slk",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	if n := strings.Count(s, "in thread"); n != 1 {
		t.Errorf("%d pins say in thread, want 1", n)
	}
}

func TestView_NoLegendWithoutAnSlkBookmark(t *testing.T) {
	m := &Model{}
	m.Open("#x", Pins)
	setLists(m, nil, []BookmarkRow{{Title: "Site", Site: "example.com"}})
	if s := screen(m); strings.Contains(s, "opens in slk") {
		t.Errorf("legend without an [slk] row:\n%s", s)
	}
}

func TestView_States(t *testing.T) {
	m := &Model{}
	m.Open("#design", Pins)
	setLists(m, nil, nil)
	s := screen(m)
	for _, want := range []string{"Nothing is pinned in #design.", "No bookmarks in #design.", "Pins and bookmarks added in the Slack app show up here.", "esc close"} {
		if !strings.Contains(s, want) {
			t.Errorf("empty state lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "j/k move") {
		t.Errorf("empty state offers moves:\n%s", s)
	}

	m.SetLoading(Pins)
	m.SetLoading(Bookmarks)
	if s := screen(m); !strings.Contains(s, "Loading pins and bookmarks…") || strings.Contains(s, "Nothing is pinned") {
		t.Errorf("loading state:\n%s", s)
	}
	if got := press(m, "enter"); got.Kind != None {
		t.Errorf("enter while loading: %+v", got)
	}
	if got := press(m, "r"); got.Kind != None {
		t.Errorf("r without an error: %+v", got)
	}

	const limited = "Slack is limiting requests right now. Wait a few seconds, then press r to retry."
	m.SetError(Pins, limited)
	m.SetError(Bookmarks, limited)
	s = screen(m)
	for _, want := range []string{"! Couldn't load pins and bookmarks for #design", "Slack is limiting requests right now.", "r retry  esc close"} {
		if !strings.Contains(s, want) {
			t.Errorf("error state lacks %q:\n%s", want, s)
		}
	}
	if got := press(m, "r"); got.Kind != Retry {
		t.Errorf("r on an error: %+v", got)
	}
}

// One list failing leaves the other on show, to move through and open.
func TestView_OneListFailed(t *testing.T) {
	m := &Model{}
	m.Open("#design", Pins)
	m.SetError(Pins, "pins.list: channel_not_found")
	m.SetBookmarks([]BookmarkRow{{Title: "Runbook", Site: "wiki.acme.com"}})
	s := screen(m)
	for _, want := range []string{"! Couldn't load pins for #design", "pins.list: channel_not_found", "Bookmarks  1", "Runbook", "r retry  esc close"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	if got := press(m, "r"); got.Kind != Retry {
		t.Errorf("r: %+v", got)
	}
	if got := press(m, "enter"); got != (Action{Kind: Open, Section: Bookmarks, Index: 0}) {
		t.Errorf("enter on the loaded list: %+v", got)
	}
}

func TestView_LongSectionScrolls(t *testing.T) {
	m := &Model{}
	m.Open("#x", Pins)
	pins := make([]PinRow, 23)
	for i := range pins {
		pins[i] = PinRow{Author: "a", Day: "Today", Text: fmt.Sprintf("pin number %02d", i+1)}
	}
	setLists(m, pins, []BookmarkRow{{Title: "Site", Site: "example.com"}})
	if s := screen(m); !strings.Contains(s, "1–10 of 23") || strings.Contains(s, "pin number 11") {
		t.Errorf("top of a long section:\n%s", s)
	}
	for range 15 {
		press(m, "j")
	}
	s := screen(m)
	if !strings.Contains(s, "7–16 of 23") || !strings.Contains(s, "pin number 16") || strings.Contains(s, "pin number 06") {
		t.Errorf("scrolled to pin 16:\n%s", s)
	}
	// A short terminal shrinks the window to fit.
	if _, h := m.BoxSize(160, 20); h > 18 {
		t.Errorf("box is %d rows on a 20-row terminal", h)
	}
}

func TestView_FolderHeading(t *testing.T) {
	m := &Model{}
	m.Open("#x", Bookmarks)
	setLists(m, nil, []BookmarkRow{
		{Title: "Runbook", Site: "wiki.acme.com"},
		{Title: "Latency", Site: "grafana.acme.com/latency", Folder: "Dashboards"},
		{Title: "Errors", Site: "grafana.acme.com/errors", Folder: "Dashboards"},
	})
	lines := strings.Split(screen(m), "\n")
	var heading, latency int
	for i, l := range lines {
		if strings.Contains(l, "Dashboards") {
			heading = i
		}
		if strings.Contains(l, "Latency") {
			latency = i
		}
	}
	if heading == 0 || latency != heading+1 || strings.Count(screen(m), "Dashboards") != 1 {
		t.Errorf("want one Dashboards heading over its links:\n%s", screen(m))
	}
	press(m, "j")
	if at(m) != "1:1" {
		t.Errorf("j from the first link should skip the heading to Latency, at %s", at(m))
	}
}

func TestClickRow(t *testing.T) {
	m := fixture()
	lines := strings.Split(screen(m), "\n")
	for y, l := range lines {
		if strings.Contains(l, "Status page") {
			if !m.ClickRow(160, 40, y) {
				t.Fatal("click on the Status page row hit nothing")
			}
			if got := press(m, "enter"); got != (Action{Kind: Open, Section: Bookmarks, Index: 2}) {
				t.Errorf("open after click: %+v", got)
			}
			break
		}
	}
	m = fixture()
	for y, l := range lines {
		if strings.Contains(l, "Pins and bookmarks ·") && m.ClickRow(160, 40, y) {
			t.Error("a click on the title hit a row")
		}
	}
}
