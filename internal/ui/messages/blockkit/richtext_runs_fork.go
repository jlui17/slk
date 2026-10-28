package blockkit

import (
	"strings"

	"github.com/slack-go/slack"
)

// Slack sends a styled phrase with inline code or a link in it (a struck
// sentence naming a `field` or linking a PR) as one element per run,
// each carrying the phrase's style. One marker pair per element
// ("~keep ~~`x`~~ y~") gives the renderer markers it can't pair, so
// neighbouring text and link elements that share bold, italic and strike
// go out as one run inside one pair ("~keep `x` y~", "~for <url|#1> are~").
func styleRuns(elements []slack.RichTextSectionElement) [][]slack.RichTextSectionElement {
	var runs [][]slack.RichTextSectionElement
	for i, e := range elements {
		if i > 0 && sameMarkerStyle(elements[i-1], e) {
			runs[len(runs)-1] = append(runs[len(runs)-1], e)
			continue
		}
		runs = append(runs, []slack.RichTextSectionElement{e})
	}
	return runs
}

func runToMrkdwn(run []slack.RichTextSectionElement) string {
	style, ok := markerStyle(run[0])
	if !ok {
		return inlineToMrkdwn(run[0])
	}
	var body strings.Builder
	for _, e := range run {
		switch v := e.(type) {
		case *slack.RichTextSectionTextElement:
			text := escapeLiteralText(v.Text)
			if v.Style.Code {
				text = "`" + text + "`"
			}
			body.WriteString(text)
		case *slack.RichTextSectionLinkElement:
			body.WriteString(linkToMrkdwn(v.URL, v.Text))
		}
	}
	out := body.String()
	if style.Strike {
		out = "~" + out + "~"
	}
	if style.Italic {
		out = "_" + out + "_"
	}
	if style.Bold {
		out = "*" + out + "*"
	}
	return out
}

func sameMarkerStyle(a, b slack.RichTextSectionElement) bool {
	sa, okA := markerStyle(a)
	sb, okB := markerStyle(b)
	return okA && okB && sa == sb
}

// markerStyle is the bold, italic and strike of a text or link element
// that can sit inside a marker pair: one on a single line, with a marker
// to share.
func markerStyle(e slack.RichTextSectionElement) (slack.RichTextSectionTextStyle, bool) {
	var text string
	var style *slack.RichTextSectionTextStyle
	switch v := e.(type) {
	case *slack.RichTextSectionTextElement:
		text, style = v.Text, v.Style
	case *slack.RichTextSectionLinkElement:
		text, style = v.URL, v.Style
	}
	if style == nil || strings.Contains(text, "\n") || strings.TrimSpace(text) == "" {
		return slack.RichTextSectionTextStyle{}, false
	}
	s := slack.RichTextSectionTextStyle{Bold: style.Bold, Italic: style.Italic, Strike: style.Strike}
	return s, s != slack.RichTextSectionTextStyle{}
}
