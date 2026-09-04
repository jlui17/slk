package blockkit

import (
	"testing"

	"github.com/slack-go/slack"
)

func TestRichTextToMrkdwn_LiteralMarkupCharsEscaped(t *testing.T) {
	rt := rtSection(&slack.RichTextSectionTextElement{
		Type: slack.RTSEText,
		Text: "ping <@U1> & open <https://x.com|y>",
	})
	want := "ping &lt;@U1&gt; &amp; open &lt;https://x.com|y&gt;"
	if got := RichTextToMrkdwn(rt); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRichTextToMrkdwn_StyledLiteralEscapedInsideMarkers(t *testing.T) {
	rt := rtSection(&slack.RichTextSectionTextElement{
		Type:  slack.RTSEText,
		Text:  "a < b",
		Style: &slack.RichTextSectionTextStyle{Bold: true},
	})
	if got, want := RichTextToMrkdwn(rt), "*a &lt; b*"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRichTextToMrkdwn_CodeLiteralEscaped(t *testing.T) {
	rt := rtSection(&slack.RichTextSectionTextElement{
		Type:  slack.RTSEText,
		Text:  "<@U1>",
		Style: &slack.RichTextSectionTextStyle{Code: true},
	})
	if got, want := RichTextToMrkdwn(rt), "`&lt;@U1&gt;`"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRichTextToMrkdwn_PreformattedLiteralEscaped(t *testing.T) {
	rt := RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextPreformatted{
			Type: slack.RTEPreformatted,
			Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "if a < b && c > d"},
			},
		},
	}}
	want := "```\nif a &lt; b &amp;&amp; c &gt; d\n```"
	if got := RichTextToMrkdwn(rt); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRichTextToMrkdwn_LiteralTextBesideRealMarkupElements(t *testing.T) {
	rt := rtSection(
		&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "<not a mention> "},
		&slack.RichTextSectionUserElement{Type: slack.RTSEUser, UserID: "U1"},
		&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: " & "},
		&slack.RichTextSectionLinkElement{Type: slack.RTSELink, URL: "https://x.com", Text: "y"},
	)
	want := "&lt;not a mention&gt; <@U1> &amp; <https://x.com|y>"
	if got := RichTextToMrkdwn(rt); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestRichTextToMrkdwn_QuoteTextLiteralEscaped(t *testing.T) {
	rt := RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextQuote{Type: slack.RTEQuote, Elements: []slack.RichTextSectionElement{
			&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "see <@U1>"},
		}},
	}}
	if got, want := RichTextToMrkdwn(rt), "> see &lt;@U1&gt;"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func rtPreformatted(language, text string) RichTextBlock {
	return RichTextBlock{Elements: []slack.RichTextElement{&slack.RichTextPreformatted{
		Type:     slack.RTEPreformatted,
		Language: language,
		Elements: []slack.RichTextSectionElement{&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: text}},
	}}}
}

func TestRichTextToMrkdwn_PreformattedCarriesOnlyKnownLanguages(t *testing.T) {
	cases := []struct{ language, want string }{
		{"go", "```go\nx := 1\n```"},
		{"", "```\nx := 1\n```"},
		{"not_a_language", "```\nx := 1\n```"},
		{"go\n```", "```\nx := 1\n```"},
	}
	for _, c := range cases {
		if got := RichTextToMrkdwn(rtPreformatted(c.language, "x := 1")); got != c.want {
			t.Errorf("language %q: got %q, want %q", c.language, got, c.want)
		}
	}
}
