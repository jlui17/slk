package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
)

// Tests for the API the App's contentPane interface relies on: ClickAt's
// result, View vs ViewBare, and SelectedMessage.

// Two days apart, so each reply after the parent gets a day divider.
const (
	apiParentTS = "1700000000.000000"
	apiReply1TS = "1700200000.000000"
	apiReply2TS = "1700400000.000000"
)

func apiThread(t *testing.T, parentTS string) *Model {
	t.Helper()
	m := New()
	m.SetThread(
		messages.MessageItem{TS: parentTS, UserID: "U1", UserName: "alice", Text: "parent", Timestamp: "1:00 PM"},
		[]messages.MessageItem{
			{TS: apiReply1TS, UserID: "U2", UserName: "bob", Text: "first reply", Timestamp: "1:01 PM"},
			{TS: apiReply2TS, UserID: "U3", UserName: "carol", Text: "second reply", Timestamp: "1:02 PM"},
		},
		"C1", apiParentTS) // the thread's ts, even when the parent item lacks one
	_ = m.View(40, 80)
	if m.list.YOffset() != 0 {
		t.Fatalf("test setup: the thread scrolled (yOffset %d)", m.list.YOffset())
	}
	return m
}

// apiRow is the pane-local y of line `within` of list entry i.
func apiRow(m *Model, i, within int) int {
	return m.chromeHeight + m.list.Offsets()[i] + within - m.list.YOffset()
}

// apiEntry finds the first entry satisfying pred and returns its index.
func apiEntry(t *testing.T, m *Model, what string, pred func(i int) bool) int {
	t.Helper()
	for i := range m.list.Entries() {
		if pred(i) {
			return i
		}
	}
	t.Fatalf("no %s entry in the thread list", what)
	return -1
}

func TestClickAt_ReportsSelectableRows(t *testing.T) {
	m := apiThread(t, apiParentTS)
	entries := m.list.Entries()
	reply := apiEntry(t, m, "reply", func(i int) bool { return entries[i].Ref == 0 })
	parent := apiEntry(t, m, "parent", func(i int) bool { return entries[i].Ref == parentSelected })

	if !m.ClickAt(apiRow(m, reply, 0)) {
		t.Error("ClickAt on a reply = false, want true")
	}
	if got, _ := m.SelectedMessage(); got.TS != apiReply1TS {
		t.Errorf("after clicking reply 1, selected %q", got.TS)
	}
	if !m.ClickAt(apiRow(m, reply, 0)) {
		t.Error("ClickAt on the already-selected reply = false, want true")
	}
	if !m.ClickAt(apiRow(m, parent, 0)) {
		t.Error("ClickAt on the parent = false, want true")
	}
	if got, _ := m.SelectedMessage(); got.TS != apiParentTS {
		t.Errorf("after clicking the parent, selected %q", got.TS)
	}
}

func TestClickAt_ReportsFalseOffSelectableRows(t *testing.T) {
	m := apiThread(t, apiParentTS)
	entries := m.list.Entries()
	parent := apiEntry(t, m, "parent", func(i int) bool { return entries[i].Ref == parentSelected })
	// Fork: decorations are Ref -1 entries: the replies line under the
	// parent, then before each reply its blank rows and day divider.
	repliesLine := apiEntry(t, m, "replies line", func(i int) bool { return i == parent+1 && entries[i].Ref == -1 })
	divider := apiEntry(t, m, "day divider", func(i int) bool {
		return entries[i].Ref == -1 && strings.Contains(ansi.Strip(entries[i].Lines[0]), "── ")
	})
	gap := apiEntry(t, m, "gap between replies", func(i int) bool {
		return i > 0 && entries[i].Ref == -1 && entries[i-1].Ref == 0
	})
	if !strings.Contains(ansi.Strip(entries[repliesLine].Lines[1]), "2 replies ─") {
		t.Fatalf("test setup: entry %d is not the replies line", repliesLine)
	}
	if strings.TrimSpace(ansi.Strip(entries[gap].Lines[0])) != "" {
		t.Fatal("test setup: the gap row is not blank")
	}

	before := m.selected
	cases := []struct {
		name string
		y    int
	}{
		{"chrome row", 0},
		{"replies line", apiRow(m, repliesLine, 1)},
		{"day divider", apiRow(m, divider, 0)},
		{"blank row between replies", apiRow(m, gap, 0)},
	}
	for _, c := range cases {
		if m.ClickAt(c.y) {
			t.Errorf("ClickAt on the %s (y=%d) = true, want false", c.name, c.y)
		}
		if m.selected != before {
			t.Errorf("ClickAt on the %s moved the cursor to %d", c.name, m.selected)
		}
	}
}

func TestClickAt_ParentWithoutTSReportsFalse(t *testing.T) {
	m := apiThread(t, "")
	entries := m.list.Entries()
	parent := apiEntry(t, m, "parent", func(i int) bool { return entries[i].Ref == parentSelected })
	if m.ClickAt(apiRow(m, parent, 0)) {
		t.Error("ClickAt on a parent with no ts = true, want false")
	}
}

func TestViewBare_OmitsTheSelectionHighlight(t *testing.T) {
	m := apiThread(t, apiParentTS)
	entries := m.list.Entries()
	reply := apiEntry(t, m, "reply", func(i int) bool { return entries[i].Ref == 0 })
	y := apiRow(m, reply, 0)
	m.BeginSelectionAt(y, 2)
	m.ExtendSelectionAt(y, 20)
	if !m.HasSelection() {
		t.Fatal("test setup: no selection")
	}

	view := m.View(40, 80)
	bare := m.ViewBare(40, 80)
	if view == bare {
		t.Fatal("View and ViewBare are identical with an active selection; View must paint the highlight")
	}
	// The highlight is the only difference: the text is the same.
	if ansi.Strip(view) != ansi.Strip(bare) {
		t.Error("View and ViewBare differ in text, not just in the highlight")
	}
	// Without a selection there is nothing to paint.
	m.ClearSelection()
	if v, b := m.View(40, 80), m.ViewBare(40, 80); v != b {
		t.Error("View and ViewBare differ without a selection")
	}
}

func TestSelectedMessage(t *testing.T) {
	if _, ok := New().SelectedMessage(); ok {
		t.Error("SelectedMessage on a model with no thread open: ok = true")
	}

	m := apiThread(t, apiParentTS)
	m.SelectByIndex(1)
	if got, ok := m.SelectedMessage(); !ok || got.TS != apiReply2TS {
		t.Errorf("reply selected: SelectedMessage = %q, %v; want %q, true", got.TS, ok, apiReply2TS)
	}
	m.GoToTop()
	m.MoveUp()
	if got, ok := m.SelectedMessage(); !ok || got.TS != apiParentTS {
		t.Errorf("parent selected: SelectedMessage = %q, %v; want %q, true", got.TS, ok, apiParentTS)
	}

	m.Clear()
	if _, ok := m.SelectedMessage(); ok {
		t.Error("SelectedMessage after Clear: ok = true")
	}
}
