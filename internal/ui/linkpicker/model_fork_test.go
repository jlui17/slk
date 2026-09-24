package linkpicker

import (
	"slices"
	"testing"
)

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

func markedURLs(m *Model) []string {
	var urls []string
	for _, it := range m.Marked() {
		urls = append(urls, it.URL)
	}
	return urls
}

func TestToggleMark_MarkedInListOrder(t *testing.T) {
	m := New()
	m.Open("Open link in herdr tab", items3())
	m.SetMultiSelect(true)
	// Mark the last row first: Marked follows list order, not mark order.
	m.HandleKey("j")
	m.HandleKey("j")
	m.ToggleMark()
	m.HandleKey("k")
	m.HandleKey("k")
	m.ToggleMark()
	want := []string{items3()[0].URL, items3()[2].URL}
	if got := markedURLs(m); !slices.Equal(got, want) {
		t.Errorf("Marked = %v, want %v", got, want)
	}
	m.ToggleMark()
	if got := markedURLs(m); !slices.Equal(got, want[1:]) {
		t.Errorf("Marked after unmarking row 0 = %v, want %v", got, want[1:])
	}
}

func TestToggleMarkAll(t *testing.T) {
	tests := []struct {
		name       string
		premarked  int // rows marked from the top before ToggleMarkAll
		wantMarked int
	}{
		{"none marked marks all", 0, 3},
		{"some marked marks all", 2, 3},
		{"all marked clears all", 3, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.Open("Open link in herdr tab", items3())
			m.SetMultiSelect(true)
			for i := 0; i < tt.premarked; i++ {
				m.ToggleMark()
				m.HandleKey("j")
			}
			m.ToggleMarkAll()
			if got := len(m.Marked()); got != tt.wantMarked {
				t.Errorf("len(Marked) = %d, want %d", got, tt.wantMarked)
			}
		})
	}
}

func TestOpenAndCloseResetMultiSelect(t *testing.T) {
	tests := []struct {
		name  string
		reset func(m *Model)
	}{
		{"Close", func(m *Model) { m.Close() }},
		{"Open", func(m *Model) { m.Open("Open link", items3()) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New()
			m.Open("Open link in herdr tab", items3())
			m.SetMultiSelect(true)
			m.ToggleMark()
			tt.reset(m)
			if m.MultiSelect() {
				t.Error("MultiSelect survived")
			}
			if got := m.Marked(); len(got) != 0 {
				t.Errorf("Marked = %v, want none", got)
			}
		})
	}
}
