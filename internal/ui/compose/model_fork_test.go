package compose

import (
	"strings"
	"testing"
)

func TestCursorOnFirstVisualRow(t *testing.T) {
	m := New("general")
	m.SetWidth(30)
	m.Focus()
	if !m.CursorOnFirstVisualRow() {
		t.Fatal("an empty box has its cursor on the first row")
	}

	// One logical line that wraps: the cursor ends up on a lower row
	// of line 0.
	m.SetValue(strings.Repeat("word ", 20))
	if !m.CursorAtFirstLine() {
		t.Fatal("setup: the cursor should still be on logical line 0")
	}
	if m.CursorOnFirstVisualRow() {
		t.Fatal("the cursor is on a wrapped row of the first line, not on the first row")
	}
	m.MoveCursorToStart()
	if !m.CursorOnFirstVisualRow() {
		t.Fatal("the start of the text is on the first row")
	}

	m.SetValue("one\ntwo")
	if m.CursorOnFirstVisualRow() {
		t.Fatal("the cursor is on the second line")
	}
}

func TestRemoveAttachmentAt(t *testing.T) {
	m := New("general")
	for _, name := range []string{"a.png", "b.png", "c.png"} {
		m.AddAttachment(PendingAttachment{Filename: name})
	}
	before := m.Version()

	m.RemoveAttachmentAt(1)

	var names []string
	for _, a := range m.Attachments() {
		names = append(names, a.Filename)
	}
	if got := strings.Join(names, ","); got != "a.png,c.png" {
		t.Fatalf("attachments = %s, want a.png,c.png", got)
	}
	if m.Version() == before {
		t.Fatal("removing an attachment must bump the version so the box redraws")
	}
}
