package syntax

import (
	"image/color"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

func withTheme(t *testing.T, name string) {
	t.Helper()
	styles.Apply(name, config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })
}

func TestLexer_NamesAndAliasesOnly(t *testing.T) {
	for lang, want := range map[string]bool{"go": true, "Go": true, "golang": true, "sh": true, "main.go": false, "": false, "go\n```": false, "nope": false} {
		if got := Lexer(lang) != nil; got != want {
			t.Errorf("Lexer(%q) != nil = %v, want %v", lang, got, want)
		}
	}
	if got := Lexer("json").Config().Name; got != "JSON" {
		t.Errorf("display name = %q, want JSON", got)
	}
}

func TestHighlight_PaintsFromThemeAndKeepsTextIntact(t *testing.T) {
	withTheme(t, "dark")
	const code = "func main() { s := \"x\" // hi\n}"
	spans := Highlight(code, Lexer("go"))
	var text strings.Builder
	got := map[string]color.Color{}
	for _, s := range spans {
		text.WriteString(s.Text)
		got[strings.TrimSpace(s.Text)] = s.Color
	}
	if text.String() != code {
		t.Fatalf("text changed: %q", text.String())
	}
	for tok, want := range map[string]color.Color{"func": styles.Primary, "\"x\"": styles.Accent, "// hi": styles.TextMuted, "main": styles.TextPrimary} {
		if got[tok] != want {
			t.Errorf("%q colored %v, want %v", tok, got[tok], want)
		}
	}
}

func TestHighlight_FollowsThemeChanges(t *testing.T) {
	withTheme(t, "dark")
	dark := Highlight("func", Lexer("go"))[0].Color
	withTheme(t, "dracula")
	if dracula := Highlight("func", Lexer("go"))[0].Color; dracula == dark || dracula != styles.Primary {
		t.Errorf("keyword color did not follow the theme: dark=%v dracula=%v", dark, dracula)
	}
}

func TestHighlight_DropsTheNewlineEnsureNLLexersAppend(t *testing.T) {
	for lang, code := range map[string]string{"diff": "-a\n+b", "c": "int x;\n// done", "go": "x := 1"} {
		var text strings.Builder
		for _, s := range Highlight(code, Lexer(lang)) {
			text.WriteString(s.Text)
		}
		if text.String() != code {
			t.Errorf("%s: text %q, want %q", lang, text.String(), code)
		}
	}
}
