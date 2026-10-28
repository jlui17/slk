package messages

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A bold or italic phrase with code, a link or a mention in it stays bold
// or italic after that span, as Slack draws it. "*keep `x` y*" is the
// shape styleRuns gives a bold phrase that names a `field`, and "*_x_*"
// the one sectionToMrkdwn gives bold italic text.
func TestStyleSpans_OuterStyleSurvivesInnerSpans(t *testing.T) {
	const bold, italic = "1/22", "3/23"
	cases := []struct{ in, attr, want string }{
		{"*keep `x` y*", bold, "keep x y"},
		{"*see <https://x.io|link> now*", bold, "see link now"},
		{"_see <@U1> now_", italic, "see @Bob now"},
		{"*_both_*", bold, "both"},
		{"*_both_*", italic, "both"},
		{"_*both*_", italic, "both"},
	}
	for _, tc := range cases {
		out := RenderSlackMarkdown(tc.in, map[string]string{"U1": "Bob"}, nil)
		on, off := tc.attr[:1], tc.attr[2:]
		if plain, styled := ansi.Strip(out), sgrAttrText(out, on, off); plain != tc.want || styled != tc.want {
			t.Errorf("RenderSlackMarkdown(%q):\n drawn         %q\n with SGR %-4s %q\n want both %q\nraw=%q", tc.in, plain, on, styled, tc.want, out)
		}
	}
}
