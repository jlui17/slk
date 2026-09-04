package syntax

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

func withTheme(t *testing.T, name string) {
	t.Helper()
	styles.Apply(name, config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
}

func fgOf(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r>>8, g>>8, b>>8)
}

func TestHighlight_PaintsTokensFromThemeAndKeepsTextLiteral(t *testing.T) {
	withTheme(t, "dark")
	const code = "func main() { s := \"x\" // hi\n}"
	out := Highlight(code, "go")
	if ansi.Strip(out) != code {
		t.Fatalf("visible text changed: %q", ansi.Strip(out))
	}
	for _, want := range []string{
		fgOf(styles.Primary) + "func",
		fgOf(styles.Accent) + "\"x\"",
		fgOf(styles.TextMuted) + "// hi",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestHighlight_EmitsNoResets(t *testing.T) {
	out := Highlight("x = \"a\" # c\n1", "python")
	if strings.Contains(out, "\x1b[0m") || strings.Contains(out, "\x1b[m") {
		t.Errorf("highlight emitted a reset: %q", out)
	}
}

func TestHighlight_UnknownLanguageIsPassthrough(t *testing.T) {
	if out := Highlight("x", "not-a-language"); out != "x" {
		t.Errorf("got %q", out)
	}
	if Known("not-a-language") || !Known("go") {
		t.Error("Known disagrees with chroma's lexer table")
	}
}

func TestHighlight_FollowsThemeChanges(t *testing.T) {
	withTheme(t, "dark")
	dark := Highlight("func", "go")
	withTheme(t, "dracula")
	dracula := Highlight("func", "go")
	if dark == dracula || !strings.HasPrefix(dracula, fgOf(styles.Primary)) {
		t.Errorf("keyword color did not follow the theme: dark=%q dracula=%q", dark, dracula)
	}
}

func TestName_IsTheLexerDisplayName(t *testing.T) {
	for lang, want := range map[string]string{"go": "Go", "json": "JSON", "bash": "Bash", "nope": ""} {
		if got := Name(lang); got != want {
			t.Errorf("Name(%q) = %q, want %q", lang, got, want)
		}
	}
}
