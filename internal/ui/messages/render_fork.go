package messages

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var listItemRe = regexp.MustCompile(`^ *(• |\d+\. )`)

type searchHighlighter struct {
	terms      []string
	start, end string
}

func newSearchHighlighter(terms []string) searchHighlighter {
	if len(terms) == 0 {
		return searchHighlighter{}
	}
	start, end, ok := SearchHighlightSGR()
	if !ok {
		return searchHighlighter{}
	}
	return searchHighlighter{terms: terms, start: start, end: end}
}

func (h searchHighlighter) highlight(s string) string {
	return HighlightSearchTerms(s, h.terms, h.start, h.end)
}

// lipgloss styles every row as open+row+close, so a highlight still open
// at a row's end is cancelled by that row's close: re-open it on the rows
// that follow. Requires a non-zero highlighter.
func (h searchHighlighter) reopenAcrossRows(rows string) string {
	split := strings.Split(rows, "\n")
	open := false
	for i, row := range split {
		if open {
			split[i] = h.start + row
		}
		if a, b := strings.LastIndex(row, h.start), strings.LastIndex(row, h.end); a >= 0 || b >= 0 {
			open = a > b
		}
	}
	return strings.Join(split, "\n")
}

func stripQuotePrefix(line string) (string, bool) {
	body := strings.TrimPrefix(line, "&gt; ")
	body = strings.TrimPrefix(body, "> ")
	return body, body != line
}

// Decode entities only after the markup regexes have consumed real <...>
// tokens, so escaped user input (a literal "<@U1>") never becomes a mention.
func renderInlineLine(text string, opts RenderSlackMarkdownOpts, hl searchHighlighter) string {
	return hl.highlight(slackEntityDecoder.Replace(renderInlineFormattingWith(text, opts)))
}

func renderBlockquote(body string, width int, hl searchHighlighter) string {
	style := blockquoteStyle()
	body = ReapplyBgAfterResets(body, fgANSIFor(style.GetForeground()))
	body = WordWrap(body, width-style.GetHorizontalFrameSize())
	if len(hl.terms) > 0 {
		body = hl.reopenAcrossRows(body)
	}
	return style.Render(body)
}

func renderCodeBlock(inner string, width int) string {
	style := codeBlockStyle()
	inner = expandTabs(slackEntityDecoder.Replace(inner))
	return style.Render(ansi.Hardwrap(inner, width-style.GetHorizontalFrameSize(), true))
}

func renderListItem(line string, opts RenderSlackMarkdownOpts, hl searchHighlighter) (string, bool) {
	head := listItemRe.FindString(line)
	if head == "" {
		return "", false
	}
	body := renderInlineLine(line[len(head):], opts, hl)
	w := lipgloss.Width(head)
	body = WordWrap(body, opts.Width-w)
	return head + strings.ReplaceAll(body, "\n", "\n"+strings.Repeat(" ", w)), true
}

func writeIfFits(buf *strings.Builder, line string, limit int) bool {
	line = expandTabs(line)
	if lipgloss.Width(line) > limit {
		return false
	}
	buf.WriteString(line)
	return true
}

// lipgloss paints a tab as four cells but lipgloss.Width counts it as zero.
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}
