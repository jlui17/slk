package blockkit

import (
	"encoding/json"
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

func TestRichTextToMrkdwn_PreformattedCarriesFenceSafeLanguages(t *testing.T) {
	cases := []struct{ language, want string }{
		{"go", "```<go>\nx := 1\n```"},
		{"plain_text", "```<plain_text>\nx := 1\n```"},
		{"", "```\nx := 1\n```"},
		{"go\n```", "```\nx := 1\n```"},
		{"<go>", "```\nx := 1\n```"},
	}
	for _, c := range cases {
		if got := RichTextToMrkdwn(rtPreformatted(c.language, "x := 1")); got != c.want {
			t.Errorf("language %q: got %q, want %q", c.language, got, c.want)
		}
	}
}

// The JSON round-trip is the point: slack-go must still hand
// message_mention over as an unknown element with Raw populated.
// Shape as seen 2026-09 in Claude-in-Slack replies.
func TestRichTextToMrkdwn_MessageMentionIsABareLink(t *testing.T) {
	const raw = `{"blocks":[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[
		{"type":"message_mention","message_ts":"1788296622.155919","channel_id":"C1",
		 "url":"https://x.slack.com/archives/C1/p1788296622155919"}]}]}]}`
	var p fixturePayload
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	rt := Parse(p.Blocks)[0].(RichTextBlock)
	if got, want := RichTextToMrkdwn(rt), "<https://x.slack.com/archives/C1/p1788296622155919>"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
