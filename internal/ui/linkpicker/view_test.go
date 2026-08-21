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

// A fallback row shows its raw URL as a muted suffix so two links
// with identical decoded text stay tellable apart; a preview fill
// (SetDisplay) drops the suffix.
func TestRenderBox_DetailSuffixUntilPreview(t *testing.T) {
	m := New()
	m.Open("Open link", []Item{
		{URL: "https://other.slack.com/archives/C1/p1700000000000001", Display: "other.slack.com · Today", Detail: "https://other.slack.com/archives/C1/p1700000000000001"},
	})
	if out := m.renderBox(90); !strings.Contains(out, "https://other") {
		t.Error("fallback row missing its muted URL suffix")
	}
	m.SetDisplay(0, "#general · matt: hi")
	if out := m.renderBox(90); strings.Contains(out, "https://other") {
		t.Error("preview-filled row still shows the URL suffix")
	}
}
