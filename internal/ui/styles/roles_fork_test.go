package styles

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"charm.land/lipgloss/v2"
	toml "github.com/pelletier/go-toml/v2"

	"github.com/gammons/slk/internal/core"
)

func sameColor(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

// A theme that sets no role renders every role in the color slk used for
// that job before the roles existed.
func TestRolesFallBackToTodaysColors(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })
	for key, th := range builtinThemes {
		if th.Colors.ThemeRoles != (ThemeRoles{}) {
			continue // a theme that sets its own roles (coffee)
		}
		Apply(key, core.Theme{})
		for _, c := range []struct {
			role     string
			got, was color.Color
		}{
			{"name", NameColor, Primary},
			{"text_body", TextBody, TextPrimary},
			{"code", CodeText, Warning},
			{"code_background", CodeBackground, Surface},
			{"focus_border", FocusBorderColor, Primary},
			{"new_messages", NewMessages, Error},
			{"online", Online, Accent},
			{"time", TimeColor, TextMuted},
		} {
			if !sameColor(c.got, c.was) {
				t.Errorf("%s: %s = %v, want the fallback %v", key, c.role, c.got, c.was)
			}
		}
		// The styles built from the roles keep today's colors too.
		if !sameColor(Username("U1", false).GetForeground(), Primary) {
			t.Errorf("%s: names are not in primary", key)
		}
		if !sameColor(MessageText.GetForeground(), TextPrimary) {
			t.Errorf("%s: body text is not in text", key)
		}
		if !sameColor(FocusedBorder.GetBorderTopForeground(), Primary) {
			t.Errorf("%s: the focus border is not in primary", key)
		}
		if !sameColor(PresenceOnline.GetForeground(), Accent) {
			t.Errorf("%s: online is not in accent", key)
		}
	}
}

// A fallback follows an override of its base color, and a role set in
// config beats the theme's own.
func TestRolesPrecedence(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })

	Apply("nord", core.Theme{Primary: "#123456"})
	if !sameColor(NameColor, lipgloss.Color("#123456")) {
		t.Errorf("name = %v, want the overridden primary", NameColor)
	}

	Apply("coffee", core.Theme{ThemeRoles: core.ThemeRoles{Name: "#ABCDEF"}})
	if !sameColor(NameColor, lipgloss.Color("#ABCDEF")) {
		t.Errorf("name = %v, want the config override", NameColor)
	}
	if !sameColor(TextBody, lipgloss.Color("#D2C8B6")) {
		t.Errorf("text_body = %v, want coffee's own", TextBody)
	}
}

// The role keys read from a theme file's [colors] and from config's
// [theme] table.
func TestRolesReadFromTOML(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })
	dir := t.TempDir()
	file := []byte(`
name = "Roles"

[colors]
primary = "#111111"
text_body = "#222222"
time = "#333333"
`)
	if err := os.WriteFile(filepath.Join(dir, "roles.toml"), file, 0o644); err != nil {
		t.Fatal(err)
	}
	LoadCustomThemes(os.DirFS(dir))
	Apply("roles", core.Theme{})
	if !sameColor(TextBody, lipgloss.Color("#222222")) || !sameColor(TimeColor, lipgloss.Color("#333333")) {
		t.Errorf("theme file roles not applied: text_body=%v time=%v", TextBody, TimeColor)
	}

	var cfg struct {
		Theme core.Theme `toml:"theme"`
	}
	if err := toml.Unmarshal([]byte("[theme]\nonline = \"#58AAF1\"\nprimary = \"#000001\"\n"), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Theme.Online != "#58AAF1" || cfg.Theme.Primary != "#000001" {
		t.Errorf("config [theme] = %+v", cfg.Theme)
	}
}

// Every text color coffee draws is at least 4.5:1 on what it sits on.
func TestCoffeeTextContrast(t *testing.T) {
	t.Cleanup(func() { Apply("dark", core.Theme{}) })
	Apply("coffee", core.Theme{})
	if CurrentTheme() != "Coffee" {
		t.Fatalf("theme = %q, want Coffee", CurrentTheme())
	}
	for _, c := range []struct {
		what   string
		fg, bg color.Color
	}{
		{"text", TextPrimary, Background},
		{"name", NameColor, Background},
		{"body", TextBody, Background},
		{"muted", TextMuted, Background},
		{"time", TimeColor, Background},
		{"link", Primary, Background},
		{"accent", Accent, Background},
		{"new messages", NewMessages, Background},
		{"code", CodeText, CodeBackground},
		{"code block language", TextBody, CodeBackground},
		{"code block copy", TimeColor, CodeBackground},
		{"code block comment", TextMuted, CodeBackground},
		{"code block deletion", Error, CodeBackground},
		{"code block link", Primary, CodeBackground},
		{"code block accent", Accent, CodeBackground},
		{"name on the focused selection", NameColor, SelectionTintColor(true)},
		{"name on the unfocused selection", NameColor, SelectionTintColor(false)},
		{"time on the focused selection", TimeColor, SelectionTintColor(true)},
		{"time on the unfocused selection", TimeColor, SelectionTintColor(false)},
		{"muted on the focused selection", TextMuted, SelectionTintColor(true)},
		{"muted on the unfocused selection", TextMuted, SelectionTintColor(false)},
		{"body on the focused selection", TextBody, SelectionTintColor(true)},
		{"body on the unfocused selection", TextBody, SelectionTintColor(false)},
		{"status bar text", TextPrimary, SurfaceDark},
		{"status bar muted", TextMuted, SurfaceDark},
		{"connected", Online, SurfaceDark},
		{"online in the sidebar", Online, SidebarBackground},
		{"online on the rail", Online, RailBackground},
		{"reaction pill", TextMuted, Surface},
		{"own reaction pill", Accent, Surface},
		{"selected text", SelectionForeground, SelectionBackground},
	} {
		if r := contrast(c.fg, c.bg); r < 4.5 {
			t.Errorf("%s: %.2f:1, want at least 4.5:1", c.what, r)
		}
	}
}
