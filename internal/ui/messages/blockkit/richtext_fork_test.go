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

// Slack links a message inline as a message_mention element (seen 2026-09
// in Claude-in-Slack replies). slack-go v0.29 has no type for it, so it
// arrives as an unknown element carrying the raw JSON.
func TestRichTextToMrkdwn_MessageMentionIsABareLink(t *testing.T) {
	const raw = `{"blocks":[{"type":"rich_text","elements":[{"type":"rich_text_section","elements":[
		{"type":"text","text":"Started it here: "},
		{"type":"message_mention","message_ts":"1788296622.155919","channel_id":"C0BCG30UGEP",
		 "url":"https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1788296622155919"},
		{"type":"text","text":" — read that"}]}]}]}`
	var msg slack.Msg
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatal(err)
	}
	blocks := Parse(msg.Blocks)
	rt, ok := blocks[0].(RichTextBlock)
	if !ok {
		t.Fatalf("Parse produced %T, want RichTextBlock", blocks[0])
	}
	want := "Started it here: <https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1788296622155919> — read that"
	if got := RichTextToMrkdwn(rt); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
