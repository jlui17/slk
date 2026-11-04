package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// hasBoldSGR and hasItalicSGR report whether out turns the attribute
// on: lipgloss emits it first, standalone or ahead of a color.
func hasBoldSGR(out string) bool {
	return strings.Contains(out, "\x1b[1m") || strings.Contains(out, "\x1b[1;")
}

func hasItalicSGR(out string) bool {
	return strings.Contains(out, "\x1b[3m") || strings.Contains(out, "\x1b[3;")
}

// The deploy bullet of a real #gitea reply: a cron expression and a
// glob-suffixed env var, each inside inline code. Slack shows every
// asterisk and no bold.
func TestEmphasis_CodeSpansKeepLiteralAsterisks(t *testing.T) {
	in := "Inside it, the four `GITEA__cron_0X2E_git_gc_repos__*` env vars are set, and `app.ini` has `[cron.git_gc_repos]` with `ENABLED = true` and `SCHEDULE = 37 10 * * *`."
	want := "Inside it, the four GITEA__cron_0X2E_git_gc_repos__* env vars are set, and app.ini has [cron.git_gc_repos] with ENABLED = true and SCHEDULE = 37 10 * * *."
	out := RenderSlackMarkdown(in, nil, nil)
	if hasBoldSGR(out) {
		t.Errorf("emitted bold SGR:\nraw=%q", out)
	}
	if plain := ansi.Strip(out); plain != want {
		t.Errorf("plain =\n%q\nwant\n%q", plain, want)
	}
}

// No Slack marker applies inside inline code.
func TestEmphasis_NoMarkersApplyInsideCode(t *testing.T) {
	for _, in := range []string{"`*bold*`", "`_italic_`", "`~struck~`", "`a *b* c` and `d* e *f`"} {
		out := RenderSlackMarkdown(in, nil, nil)
		want := strings.ReplaceAll(in, "`", "")
		if plain := ansi.Strip(out); plain != want {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want %q", in, plain, want)
		}
		if hasBoldSGR(out) || hasItalicSGR(out) || hasStrikeSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) styled code text:\nraw=%q", in, out)
		}
	}
}

// Asterisks that don't hug text on their inner sides, or that sit inside
// a word, stay literal, as in Slack.
func TestEmphasis_ProseAsterisksStayLiteral(t *testing.T) {
	for _, in := range []string{
		"the cron moved to 37 10 * * * today",
		"2 * 3 * 4",
		"a*b*c",
		"*not bold *",
		"* not bold*",
	} {
		out := RenderSlackMarkdown(in, nil, nil)
		if hasBoldSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) emitted bold SGR:\nraw=%q", in, out)
		}
		if plain := ansi.Strip(out); plain != in {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want input unchanged", in, plain)
		}
	}
}

func TestEmphasis_RealBoldStillRenders(t *testing.T) {
	cases := []struct{ in, plain string }{
		{"*bold*", "bold"},
		{"(*bold*)", "(bold)"},
		{"a *bold run* b", "a bold run b"},
		{"*Sent using* Claude", "Sent using Claude"},
		{"see `x` *then* go", "see x then go"},
		{"*see `x` here*", "see x here"},
	}
	for _, tc := range cases {
		out := RenderSlackMarkdown(tc.in, nil, nil)
		if !hasBoldSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) missing bold SGR:\nraw=%q", tc.in, out)
		}
		if plain := ansi.Strip(out); plain != tc.plain {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want %q", tc.in, plain, tc.plain)
		}
	}
}

// Underscores that don't hug text on their inner sides stay literal, as
// asterisks do.
func TestEmphasis_SpacedUnderscoresStayLiteral(t *testing.T) {
	for _, in := range []string{"a _ b _ c", "_not italic _", "_ not italic_"} {
		out := RenderSlackMarkdown(in, nil, nil)
		if hasItalicSGR(out) {
			t.Errorf("RenderSlackMarkdown(%q) emitted italic SGR:\nraw=%q", in, out)
		}
		if plain := ansi.Strip(out); plain != in {
			t.Errorf("RenderSlackMarkdown(%q) plain = %q, want input unchanged", in, plain)
		}
	}
}

// Slack's editor styles part of a word ("un" of "unbelievable") as its
// own rich-text element; slk draws that part styled, as Slack does, though
// a typed "*un*believable" stays literal.
func TestEmphasis_RichTextStylesPartOfAWord(t *testing.T) {
	text := func(s string, style *slack.RichTextSectionTextStyle) slack.RichTextSectionElement {
		return &slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: s, Style: style}
	}
	bold := &slack.RichTextSectionTextStyle{Bold: true}
	strike := &slack.RichTextSectionTextStyle{Strike: true}
	cases := []struct {
		name     string
		elements []slack.RichTextSectionElement
		styled   func(string) string
		want     string
	}{
		{"bold prefix", []slack.RichTextSectionElement{text("un", bold), text("believable", nil)}, boldText, "un"},
		{"bold middle", []slack.RichTextSectionElement{text("un", nil), text("believ", bold), text("able", nil)}, boldText, "believ"},
		{"strike prefix", []slack.RichTextSectionElement{text("un", strike), text("believable", nil)}, struckText, "un"},
		{"strike suffix", []slack.RichTextSectionElement{text("unbeliev", nil), text("able", strike)}, struckText, "able"},
	}
	for _, tc := range cases {
		rt := blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: tc.elements},
		}}
		out := RenderSlackMarkdown(blockkit.RichTextToMrkdwn(rt), nil, nil)
		if got := tc.styled(out); got != tc.want {
			t.Errorf("%s: styled %q, want %q\nraw=%q", tc.name, got, tc.want, out)
		}
		if plain := ansi.Strip(out); plain != "unbelievable" {
			t.Errorf("%s: plain %q, want %q", tc.name, plain, "unbelievable")
		}
	}
	if out := RenderSlackMarkdown("*un*believable", nil, nil); hasBoldSGR(out) {
		t.Errorf("typed *un*believable drew bold: %q", out)
	}
}

func boldText(s string) string {
	return sgrAttrText(s, "1", "22")
}

// Edit source reads the rich text through its own converter
// (RichTextToMarkdown), which writes the composer's markdown and never
// the zero-width space the mrkdwn converter adds; a partly bold word
// seeds the composer as written.
func TestEditSource_PartlyBoldWord(t *testing.T) {
	msg := MessageItem{Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "un", Style: &slack.RichTextSectionTextStyle{Bold: true}},
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "believable"},
		}},
	}}}}
	if got, want := EditSource(msg), "**un**believable"; got != want {
		t.Errorf("EditSource = %q, want %q", got, want)
	}
}

// A rich-text bold or strike run whose own text holds its marker
// ("5 * 3 = 15" bold) draws whole and styled, as Slack's editor shows it.
func TestEmphasis_RichTextRunHoldingItsMarker(t *testing.T) {
	cases := []struct {
		text   string
		style  slack.RichTextSectionTextStyle
		styled func(string) string
	}{
		{"5 * 3 = 15", slack.RichTextSectionTextStyle{Bold: true}, boldText},
		{"run rm *.tmp now", slack.RichTextSectionTextStyle{Bold: true}, boldText},
		{"wait ~5 min", slack.RichTextSectionTextStyle{Strike: true}, struckText},
		{"a ~ b", slack.RichTextSectionTextStyle{Strike: true}, struckText},
		{"snake_case", slack.RichTextSectionTextStyle{Italic: true}, italicText},
		{"rename my_var now", slack.RichTextSectionTextStyle{Italic: true}, italicText},
		{"*star", slack.RichTextSectionTextStyle{Bold: true}, boldText},
	}
	for _, tc := range cases {
		rt := blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: tc.text, Style: &tc.style},
			}},
		}}
		out := RenderSlackMarkdown(blockkit.RichTextToMrkdwn(rt), nil, nil)
		if got := tc.styled(out); got != tc.text {
			t.Errorf("%q: styled %q, want all of it\nraw=%q", tc.text, got, out)
		}
		if plain := ansi.Strip(out); plain != tc.text {
			t.Errorf("%q: plain %q", tc.text, plain)
		}
	}
}

// Typed mrkdwn carries no guard: an asterisk with a space before it ends
// the search for a closer, so these stay literal. Whether Slack bolds a
// typed "*5 * 3 = 15*" is unverified; slk keeps it literal.
func TestEmphasis_TypedInnerMarkersStayLiteral(t *testing.T) {
	for _, in := range []string{"37 10 * * *", "*5 * 3 = 15*", "~a ~ b~"} {
		out := RenderSlackMarkdown(in, nil, nil)
		if hasBoldSGR(out) || hasStrikeSGR(out) || ansi.Strip(out) != in {
			t.Errorf("RenderSlackMarkdown(%q) = %q", in, out)
		}
	}
}

func italicText(s string) string {
	return sgrAttrText(s, "3", "23")
}
