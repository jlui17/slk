package messages

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// Phase 3 characterization (plan Task 0.3): the channel pane's side of
// every divergence the shared substrate turns into an option.

func p3Messages(t *testing.T, n int, dayStep int64) *Model {
	t.Helper()
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	items := make([]MessageItem, n)
	for i := range items {
		items[i] = MessageItem{
			TS:        fmt.Sprintf("%d.000000", 1700000000+int64(i)*dayStep+int64(i)),
			UserName:  "bob",
			Text:      fmt.Sprintf("mark%03d %s", i, strings.Repeat("word ", 30)),
			Timestamp: "10:30 AM",
		}
	}
	m := New(items, "general")
	return m
}

func p3MsgRows(view string) []string {
	rows := strings.Split(view, "\n")
	for i := range rows {
		rows[i] = ansi.Strip(rows[i])
	}
	return rows
}

func p3MsgRowWith(t *testing.T, rows []string, anchor string) int {
	t.Helper()
	for i, r := range rows {
		if strings.Contains(r, anchor) {
			return i
		}
	}
	t.Fatalf("no row contains %q:\n%s", anchor, strings.Join(rows, "\n"))
	return -1
}

// U6: the channel pane clamps lazily, in View, so an overshoot below the
// bottom absorbs a following ScrollUp(1).
func TestChar_MessagesScrollClampsLazily(t *testing.T) {
	a := p3Messages(t, 40, 0)
	bottom := a.View(20, 80)
	a.ScrollDown(50)
	a.ScrollUp(1)
	if got := a.View(20, 80); got != bottom {
		t.Error("ScrollDown(50)+ScrollUp(1) left the bottom (channel pane clamps lazily, U6)")
	}
}

// U1 on: scrolling the selection off-screen drags it onto the screen.
func TestChar_MessagesCursorFollowsScroll(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	before := m.SelectedIndex()
	m.ScrollUp(200)
	_ = m.View(20, 80)
	if m.SelectedIndex() >= before {
		t.Errorf("selection did not follow the scroll: %d -> %d (U1)", before, m.SelectedIndex())
	}
}

// U2 on: a bottom-anchored pane that shrinks keeps its bottom pinned.
func TestChar_MessagesPinsBottomOnShrink(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	y0 := m.YOffset()
	_ = m.View(17, 80)
	if got := m.YOffset(); got != y0+3 {
		t.Errorf("YOffset after shrinking by 3 = %d, want %d (U2)", got, y0+3)
	}
}

// U3 on: a pane with content below draws "-- more below --" last.
func TestChar_MessagesMoreBelowRow(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	m.ScrollUp(30)
	rows := p3MsgRows(m.View(20, 80))
	if !strings.Contains(rows[len(rows)-1], "-- more below --") {
		t.Errorf("last row = %q, want the more-below indicator (U3)", rows[len(rows)-1])
	}
}

// U4 off: only the first press of a drag bumps Version.
func TestChar_MessagesDragBumpsVersionOnce(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	r := p3MsgRowWith(t, rows, "mark001")
	v0 := m.Version()
	m.BeginSelectionAt(r, 3)
	v1 := m.Version()
	m.ExtendSelectionAt(r, 10)
	v2 := m.Version()
	if _, ok := m.EndSelection(); !ok {
		t.Fatal("test setup: the drag selected nothing")
	}
	v3 := m.Version()
	if !(v1 == v0+1 && v2 == v1 && v3 == v2) {
		t.Errorf("Version over a drag = %d,%d,%d,%d; want +1 on begin only (U4)", v0, v1, v2, v3)
	}
}

// 8(c) on: a press on a day divider snaps to the next message.
func TestChar_MessagesSelectionSnapsFromDivider(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	div := -1
	for i := m.ChromeHeight(); i < len(rows); i++ {
		if strings.Contains(rows[i], "── ") && !strings.Contains(rows[i], "▌") {
			div = i
			break
		}
	}
	if div < 0 {
		t.Fatalf("test setup: no day divider:\n%s", strings.Join(rows, "\n"))
	}
	m.BeginSelectionAt(div, 3)
	if !m.HasSelection() {
		t.Error("a press on a day divider did not snap to a message (8c)")
	}
}

// 8(c) on: a copy across a day boundary includes the divider's text.
func TestChar_MessagesCopyIncludesDividerText(t *testing.T) {
	m := p3Messages(t, 2, 2*86400)
	rows := p3MsgRows(m.View(40, 80))
	m.BeginSelectionAt(p3MsgRowWith(t, rows, "mark000"), 3)
	m.ExtendSelectionAt(p3MsgRowWith(t, rows, "mark001"), 8)
	text, ok := m.EndSelection()
	if !ok || !strings.Contains(text, "──") {
		t.Errorf("copy across a day boundary = %q (ok=%v), want the divider text in it", text, ok)
	}
}

// Review focus 4: a resize during a selection keeps it.
func TestChar_MessagesSelectionSurvivesResize(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	m.BeginSelectionAt(p3MsgRowWith(t, rows, "mark000"), 3)
	m.ExtendSelectionAt(p3MsgRowWith(t, rows, "mark001"), 8)
	_ = m.View(40, 60)
	if !m.HasSelection() {
		t.Fatal("resize dropped the selection")
	}
	if got := m.SelectionText(); !strings.HasPrefix(got, "rk000") {
		t.Errorf("after resize the copy = %q, want prefix %q", got, "rk000")
	}
}

// Between a cache discard (here AppendMessage) and the next View, clicks
// and selections find no message. The substrate keeps this via
// KeepLookups(0).
func TestChar_MessagesLookupsMissUntilRebuilt(t *testing.T) {
	m := p3Messages(t, 3, 0)
	rows := p3MsgRows(m.View(40, 80))
	r := p3MsgRowWith(t, rows, "mark000")
	m.AppendMessage(MessageItem{TS: "1800000000.000000", UserName: "carol", Text: "fresh", Timestamp: "11:00 AM"})
	if m.ClickAt(r) {
		t.Error("ClickAt hit a message while the render cache was discarded")
	}
	m.BeginSelectionAt(r, 3)
	if m.HasSelection() {
		t.Error("a selection started while the render cache was discarded")
	}
}

// The channel pane's ClickAt misses below the content (thread's selects
// the last reply).
func TestChar_MessagesClickBelowContentMisses(t *testing.T) {
	m := p3Messages(t, 2, 0)
	_ = m.View(40, 80)
	m.SelectByIndex(0)
	if m.ClickAt(39) || m.SelectedIndex() != 0 {
		t.Errorf("click below the content hit (selected=%d)", m.SelectedIndex())
	}
}

// U5 (ReanchorPolicy.BottomUsesNewHeight): a frame that changes width AND
// height together decides "was at the bottom" against the new height. Three
// lines above the bottom, a taller and narrower frame is at the bottom
// under the new window but not under the old one, so the pane must end
// pinned to the bottom. The width drops far enough that the rewrapped
// content below the anchor outgrows the taller window: only the policy,
// not the lazy clamp, can pin it.
func TestChar_MessagesReanchorMeasuresBottomWithNewHeight(t *testing.T) {
	m := p3Messages(t, 40, 0)
	_ = m.View(20, 80)
	m.ScrollUp(3)
	_ = m.View(20, 80)

	y, oldH, total := m.list.YOffset(), m.list.Height(), m.list.Total()
	const newOuter = 30 // 10 rows taller than the 20 above
	newH := oldH + (newOuter - 20)
	if y+oldH >= total || y+newH < total {
		t.Fatalf("test setup: y=%d oldH=%d newH=%d total=%d; want the old window off the bottom and the new one at it", y, oldH, newH, total)
	}

	_ = m.View(newOuter, 40)

	if got, want := m.list.YOffset(), m.list.Total()-m.list.Height(); got != want {
		t.Errorf("YOffset after a taller+narrower frame = %d, want %d (bottom-pinned, U5)", got, want)
	}
}
