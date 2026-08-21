package linkpicker

import (
	"strings"
	"testing"
)

// Display replaces the URL in the rendered row; the URL stays the
// open target but must not show.
func TestRenderBox_DisplayReplacesURL(t *testing.T) {
	m := New()
	m.Open("Open link", []Item{
		{URL: "https://myteam.slack.com/archives/C1/p1700000000000001", Display: "#general · Today", InApp: true},
		{URL: "https://b.example/2"},
	})
	out := m.renderBox(80)
	if !strings.Contains(out, "#general · Today") {
		t.Error("rendered box missing Display text")
	}
	if strings.Contains(out, "archives") {
		t.Error("rendered box shows the URL despite Display")
	}
	if !strings.Contains(out, "https://b.example/2") {
		t.Error("rendered box missing URL of Display-less row")
	}
}
