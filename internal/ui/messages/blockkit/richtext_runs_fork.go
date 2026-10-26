package blockkit

import (
	"strings"

	"github.com/slack-go/slack"
)

// Slack sends a styled phrase with inline code in it (a struck sentence
// naming a `field`) as one text element per run, each carrying the
// phrase's style. One marker pair per element ("~keep ~~`x`~~ y~") gives
// the renderer markers it can't pair, so neighbouring text elements that
// share bold, italic and strike go out as one run inside one pair
// ("~keep `x` y~").
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
	if len(run) == 1 {
		return inlineToMrkdwn(run[0])
	}
	var body strings.Builder
	for _, e := range run {
		t := e.(*slack.RichTextSectionTextElement)
		if t.Style.Code {
			body.WriteString("`" + t.Text + "`")
		} else {
			body.WriteString(t.Text)
		}
	}
	style, _ := markerStyle(run[0])
	return textToMrkdwn(body.String(), &style)
}

func sameMarkerStyle(a, b slack.RichTextSectionElement) bool {
	sa, okA := markerStyle(a)
	sb, okB := markerStyle(b)
	return okA && okB && sa == sb
}

// markerStyle is the bold, italic and strike of a text element that can
// sit inside a marker pair: one on a single line, with a marker to share.
func markerStyle(e slack.RichTextSectionElement) (slack.RichTextSectionTextStyle, bool) {
	t, ok := e.(*slack.RichTextSectionTextElement)
	if !ok || t.Style == nil || strings.Contains(t.Text, "\n") || strings.TrimSpace(t.Text) == "" {
		return slack.RichTextSectionTextStyle{}, false
	}
	s := slack.RichTextSectionTextStyle{Bold: t.Style.Bold, Italic: t.Style.Italic, Strike: t.Style.Strike}
	return s, s != slack.RichTextSectionTextStyle{}
}
