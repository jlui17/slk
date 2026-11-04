package styles

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/core"
)

// pills are the styles that draw text on a filled theme color, read
// after Apply so each one reflects the current theme.
func pills() []struct {
	name  string
	style lipgloss.Style
} {
	return []struct {
		name  string
		style lipgloss.Style
	}{
		{"workspace active", WorkspaceActive},
		{"unread badge", UnreadBadge},
		{"status mode", StatusMode},
		{"status mode insert", StatusModeInsert},
	}
}

// On coffee every pill's text reads at least 4.5:1, with today's #D97B57
// error and with the lighter #E4A187.
func TestPillCoffeeContrast(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })
	for _, errColor := range []string{"#D97B57", "#E4A187"} {
		Apply("coffee", core.Theme{Error: errColor})
		for _, p := range pills() {
			if r := contrast(p.style.GetForeground(), p.style.GetBackground()); r < 4.5 {
				t.Errorf("error %s, %s: text %.2f:1, want at least 4.5:1", errColor, p.name, r)
			}
		}
	}
}

// On every built-in theme each pill's text is the better of white and
// the theme's Background against the pill's color. A theme where even
// the better one is under 4.5:1 is logged, not failed: its pill color
// is the theme's to fix.
func TestPillContrastAllThemes(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })
	for key := range builtinThemes {
		Apply(key, core.Theme{})
		for _, p := range pills() {
			bg := p.style.GetBackground()
			white := contrast(lipgloss.Color("#FFFFFF"), bg)
			best := max(white, contrast(Background, bg))
			got := contrast(p.style.GetForeground(), bg)
			if got < best {
				t.Errorf("%s, %s: text %.2f:1, want %.2f:1 (white %.2f:1)", key, p.name, got, best, white)
			}
			if best < 4.5 {
				t.Logf("%s, %s: best text is %.2f:1, under 4.5:1 (pill %v, background %v)", key, p.name, best, bg, Background)
			}
		}
	}
}
