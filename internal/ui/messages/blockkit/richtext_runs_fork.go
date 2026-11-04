package blockkit

import (
	"strings"
	"unicode"
	"unicode/utf8"

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

// ZeroWidthSpace sits between a styled run's marker and a letter or digit
// beside it, where Slack's editor styled part of a word ("*un*" then
// "believable"): a marker against a letter doesn't pair, so without it
// "*un*believable" would draw literal. It is the usual way to style part of
// a word in Slack mrkdwn. The renderer drops it once the markers are
// paired, and copy and export drop it too (messages.DropZeroWidthSpaces).
const ZeroWidthSpace = "\u200b"

// runsToMrkdwn writes each style run of a section, with a ZeroWidthSpace
// where a styled run starts or ends mid-word.
func runsToMrkdwn(elements []slack.RichTextSectionElement) string {
	var b strings.Builder
	prevStyled := false
	for _, run := range styleRuns(elements) {
		out := runToMrkdwn(run)
		_, styled := markerStyle(run[0])
		if (styled || prevStyled) && b.Len() > 0 && out != "" && markerMeetsWord(b.String(), out) {
			b.WriteString(ZeroWidthSpace)
		}
		b.WriteString(out)
		prevStyled = styled
	}
	return b.String()
}

// markerMeetsWord reports whether the join of before and after puts a
// bold, italic or strike marker right against a letter or digit.
func markerMeetsWord(before, after string) bool {
	last, _ := utf8.DecodeLastRuneInString(before)
	first, _ := utf8.DecodeRuneInString(after)
	isMarker := func(r rune) bool { return r == '*' || r == '_' || r == '~' }
	isWord := func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }
	return isMarker(last) && isWord(first) || isWord(last) && isMarker(first)
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
			} else {
				text = guardInnerMarkers(text, style)
			}
			body.WriteString(text)
		case *slack.RichTextSectionLinkElement:
			body.WriteString(linkToMrkdwn(v.URL, v.Text))
		}
	}
	// Slack's markers must hug text, so a run's edge spaces (a bold
	// "Rollout report. " ahead of plain text) go outside them.
	whole := body.String()
	text := strings.TrimLeftFunc(whole, unicode.IsSpace)
	lead := whole[:len(whole)-len(text)]
	out := strings.TrimRightFunc(text, unicode.IsSpace)
	trail := text[len(out):]
	if style.Strike {
		out = "~" + out + "~"
	}
	if style.Italic {
		out = "_" + out + "_"
	}
	if style.Bold {
		out = "*" + out + "*"
	}
	return lead + out + trail
}

// guardInnerMarkers puts a ZeroWidthSpace before each of the run's own
// markers in its text, so a bold "5 * 3 = 15" or an italic "snake_case"
// pairs only its outer markers: the renderer never closes a run on a
// guarded marker (messages.isGuardedMarker). Code is left as it is, since
// nothing pairs inside it.
func guardInnerMarkers(text string, style slack.RichTextSectionTextStyle) string {
	var pairs []string
	if style.Bold {
		pairs = append(pairs, "*", ZeroWidthSpace+"*")
	}
	if style.Italic {
		pairs = append(pairs, "_", ZeroWidthSpace+"_")
	}
	if style.Strike {
		pairs = append(pairs, "~", ZeroWidthSpace+"~")
	}
	if len(pairs) == 0 {
		return text
	}
	return strings.NewReplacer(pairs...).Replace(text)
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
