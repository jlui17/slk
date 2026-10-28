package workspacefinder

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

func TestRenderBox_FollowsTheTheme(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	m := New()
	m.SetItems([]Item{{ID: "T1", Name: "Acme", Initials: "A"}})
	m.Open()
	dark := m.renderBox(80)

	styles.Apply("light", config.Theme{})
	light := m.renderBox(80)
	if dark == light {
		t.Fatal("the open finder drew the same bytes after a theme change")
	}
	if !strings.Contains(light, messages.BgANSI()) {
		t.Error("the finder drawn after the change does not carry the new theme's background")
	}
}
