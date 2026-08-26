package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// hasStrikeSGR reports whether out contains a strikethrough SGR (the 9
// attribute, emitted either standalone "\x1b[9m" or folded into a
// combined sequence like "\x1b[38;2;r;g;b;9m").
func hasStrikeSGR(out string) bool {
	return strings.Contains(out, "\x1b[9m") || strings.Contains(out, ";9m")
}

// Prose tildes ("~45%" meaning approximately) must never pair up into a
// strikethrough run. Slack only closes a ~ when the following rune is a
// word boundary and the preceding rune is non-whitespace.
func TestStrike_ProseTildesStayLiteral(t *testing.T) {
	cases := []string{
		"went from a 421s median to ~234s measured (~45% faster), and PRs no longer burn ~80s of machine time",
		"lands ~3:20-4:00 (~45% off)",
		"cost ~$5 vs ~$10",
		"a~b~c",
	}
	for _, in := range cases {
		out := RenderSlackMarkdown(in, nil, nil)
		if hasStrikeSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) emitted strikethrough SGR:\nraw=%q", in, out)
		}
		if plain := ansi.Strip(out); plain != in {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want input unchanged", in, plain)
		}
	}
}

func TestStrike_RealStrikeStillRenders(t *testing.T) {
	cases := []struct{ in, plain string }{
		{"~done~", "done"},
		{"(~struck~)", "(struck)"},
		{"a ~struck run~ b", "a struck run b"},
	}
	for _, tc := range cases {
		out := RenderSlackMarkdown(tc.in, nil, nil)
		if !hasStrikeSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) missing strikethrough SGR:\nraw=%q", tc.in, out)
		}
		if plain := ansi.Strip(out); plain != tc.plain {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want %q", tc.in, plain, tc.plain)
		}
	}
}

// Rich-text bold+strike arrives as *~x~* (richtext.go nests bold
// outermost); the strike pass runs after bold is already ANSI-rendered,
// so the opener's preceding rune is an SGR terminator, not whitespace.
func TestStrike_NestedInBold(t *testing.T) {
	out := RenderSlackMarkdown("*bold ~struck~ bold*", nil, nil)
	if !hasStrikeSGR(out) {
		t.Errorf("strike inside bold lost: raw=%q", out)
	}
	if plain := ansi.Strip(out); plain != "bold struck bold" {
		t.Errorf("plain = %q, want %q", plain, "bold struck bold")
	}
}

func TestCommonMark_ProseTildesStayLiteral(t *testing.T) {
	in := "to ~234s measured (~45% faster), and ~80s saved"
	if got := SlackMrkdwnToCommonMark(in, nil, nil); got != in {
		t.Errorf("got %q, want input unchanged", got)
	}
}
