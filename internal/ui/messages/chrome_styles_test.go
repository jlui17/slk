package messages

import (
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

// TestChromeStyles_MatchTheInlineStyles pins each chrome style to the
// expression the dialogs built inline before Phase 4, so a dialog moved
// onto chrome renders the same bytes.
func TestChromeStyles_MatchTheInlineStyles(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	bg := styles.Background
	s := ChromeStyles()

	cases := []struct {
		name      string
		got, want lipgloss.Style
	}{
		{"Box", s.Box, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(styles.Primary).BorderBackground(bg).Background(bg).Padding(1, 1)},
		{"Popup", s.Popup, lipgloss.NewStyle().BorderStyle(lipgloss.RoundedBorder()).BorderForeground(styles.Primary).Background(styles.SurfaceDark)},
		{"Title", s.Title, lipgloss.NewStyle().Bold(true).Background(bg).Foreground(styles.Primary)},
		{"Input", s.Input, lipgloss.NewStyle().BorderStyle(lipgloss.Border{Left: "\u258c"}).BorderLeft(true).BorderForeground(styles.Primary).BorderBackground(bg).PaddingLeft(1).Background(bg).Foreground(styles.TextPrimary)},
		{"Placeholder", s.Placeholder, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted)},
		{"Row", s.Row, lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)},
		{"RowSelected", s.RowSelected, lipgloss.NewStyle().Background(bg).Foreground(styles.Primary).Bold(true)},
		{"Marker", s.Marker, lipgloss.NewStyle().Background(bg).Foreground(styles.Accent)},
		{"PopupMarker", s.PopupMarker, lipgloss.NewStyle().Foreground(styles.Accent)},
		{"Empty", s.Empty, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Italic(true)},
		{"Footer", s.Footer, lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted)},
		{"Thumb", s.Thumb, lipgloss.NewStyle().Background(bg).Foreground(styles.Primary)},
		{"Track", s.Track, lipgloss.NewStyle().Background(bg).Foreground(styles.Border)},
	}
	for _, c := range cases {
		got := c.got.Width(12).Render("ab\ncd")
		want := c.want.Width(12).Render("ab\ncd")
		if got != want {
			t.Errorf("%s renders %q, want %q", c.name, got, want)
		}
	}
	if want := BgANSI() + FgANSI(); s.BaseANSI != want {
		t.Errorf("BaseANSI = %q, want %q", s.BaseANSI, want)
	}
}

// TestChromeStyles_FollowTheTheme: built per call, so a dialog drawn
// after a theme change draws in the new theme with nothing pushed.
func TestChromeStyles_FollowTheTheme(t *testing.T) {
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
	styles.Apply("dark", config.Theme{})
	dark := ChromeStyles()
	styles.Apply("light", config.Theme{})
	light := ChromeStyles()
	if dark.Title.Render("x") == light.Title.Render("x") {
		t.Error("Title did not change with the theme")
	}
	if dark.BaseANSI == light.BaseANSI {
		t.Error("BaseANSI did not change with the theme")
	}
}
