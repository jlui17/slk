package appshortcuts

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// The shortcut apps.actions.list returned in the captured workspace.
var capturedShortcuts = []Shortcut{{AppID: "A00000APP01", AppName: "Colony", ActionID: "10000000000001", Name: "Annotate"}}

func TestMenu_Rows(t *testing.T) {
	var m Model
	m.OpenMenu(append(capturedShortcuts, Shortcut{AppName: "Jira Cloud", Name: "Create issue"}))
	box := ansi.Strip(m.box(100, 30))
	if !strings.Contains(box, "Message actions") {
		t.Errorf("no title:\n%s", box)
	}
	// Names in one column, the app after it.
	if !strings.Contains(box, "▌ Annotate       Colony") || !strings.Contains(box, "  Create issue   Jira Cloud") {
		t.Errorf("rows:\n%s", box)
	}
	if sc, ok := m.MenuKey("j", 30); ok || sc.Name != "" {
		t.Error("j should only move")
	}
	if sc, ok := m.MenuKey("enter", 30); !ok || sc.Name != "Create issue" {
		t.Errorf("enter after j = %+v %v", sc, ok)
	}
	m.MenuKey("k", 30)
	if sc, _ := m.MenuKey("enter", 30); sc != capturedShortcuts[0] {
		t.Errorf("enter after k = %+v", sc)
	}
}

func TestMenu_States(t *testing.T) {
	var m Model
	m.OpenLoading()
	if !strings.Contains(ansi.Strip(m.box(80, 30)), "Loading shortcuts…") {
		t.Error("loading text missing")
	}
	if _, ok := m.MenuKey("enter", 30); ok {
		t.Error("enter while loading ran something")
	}
	m.SetShortcuts(nil)
	if !strings.Contains(ansi.Strip(m.box(80, 30)), "No app shortcuts in this workspace") {
		t.Error("empty text missing")
	}
	m.OpenMenu(capturedShortcuts)
	m.Wait("Colony")
	if !strings.Contains(ansi.Strip(m.box(80, 30)), "Waiting for Colony…") {
		t.Error("waiting text missing")
	}
	m.Close()
	if m.IsVisible() || m.ViewOverlay(80, 30, "bg") != "bg" {
		t.Error("closed overlay still draws")
	}
}

// A keycap emoji is two cells to lipgloss and one to x/ansi; every
// row must still come out exactly as wide as the box.
func TestMenu_KeycapNameKeepsBoxWidth(t *testing.T) {
	var m Model
	m.OpenMenu([]Shortcut{{AppName: "Tracker #️⃣", Name: "1️⃣ Create a ticket in the tracker for this"}, capturedShortcuts[0]})
	for w := 12; w <= 100; w++ {
		for i, line := range strings.Split(m.box(w, 30), "\n") {
			if got := lipgloss.Width(line); got != boxWidth(w) {
				t.Fatalf("terminal %d: row %d is %d cells, box %d", w, i, got, boxWidth(w))
			}
		}
	}
}

func TestMenu_ScrollsToSelection(t *testing.T) {
	var list []Shortcut
	for i := 1; i <= 40; i++ {
		list = append(list, Shortcut{AppName: "App", Name: fmt.Sprintf("Shortcut %02d", i)})
	}
	var m Model
	m.OpenMenu(list)
	const termH = 16 // room for 10 rows
	for i := 0; i < 30; i++ {
		m.MenuKey("j", termH)
	}
	box := ansi.Strip(m.box(80, termH))
	if !strings.Contains(box, "▌ Shortcut 31") || strings.Contains(box, "Shortcut 01") {
		t.Errorf("selection not in view:\n%s", box)
	}
	if !strings.Contains(box, "22–31 of 40") {
		t.Errorf("no position in the title:\n%s", box)
	}
	if h := strings.Count(box, "\n") + 1; h > termH {
		t.Errorf("box %d rows, terminal %d", h, termH)
	}
	for i := 0; i < 5; i++ {
		m.MenuKey("k", termH)
	}
	// Moving up inside the window doesn't scroll it.
	if box := ansi.Strip(m.box(80, termH)); !strings.Contains(box, "22–31 of 40") || !strings.Contains(box, "▌ Shortcut 26") {
		t.Errorf("after k×5:\n%s", box)
	}
	short := Model{}
	short.OpenMenu(capturedShortcuts)
	if strings.Contains(ansi.Strip(short.box(80, termH)), " of ") {
		t.Error("a menu that fits shows a position")
	}
}
