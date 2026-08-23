package linkpicker

import "testing"

func TestSetDisplay(t *testing.T) {
	m := New()
	m.Open("Open link", items3())
	m.SetDisplay(2, "#general · Today")
	if got := m.Items()[2].Display; got != "#general · Today" {
		t.Errorf("Display = %q", got)
	}
	m.SetDisplay(-1, "x")
	m.SetDisplay(3, "x")
	for i, it := range m.Items()[:2] {
		if it.Display != "" {
			t.Errorf("item %d Display = %q, want empty", i, it.Display)
		}
	}
}
