package messages

import (
	"fmt"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/syntax"
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

type heldCodeBlocks []string

func (h *heldCodeBlocks) hold(rendered string) string {
	*h = append(*h, rendered)
	return fmt.Sprintf("\x00CB%d\x00", len(*h)-1)
}

func (h heldCodeBlocks) restore(s string) string {
	for i, block := range h {
		s = strings.Replace(s, fmt.Sprintf("\x00CB%d\x00", i), block, 1)
	}
	return s
}

func renderCodeBlock(inner string, width int, hl searchHighlighter) string {
	style := codeBlockStyle()
	language, code := splitFenceLanguage(slackEntityDecoder.Replace(inner))
	code = syntax.Highlight(expandTabs(code), language)
	code = ansi.Hardwrap(code, width-style.GetHorizontalFrameSize(), true)
	return style.Render(hl.highlight(reopenFgAcrossRows(code)))
}

// A first line is a language tag only when the highlighter knows it;
// otherwise it is the first line of code (a bot's "ERROR" header stays).
func splitFenceLanguage(inner string) (language, code string) {
	first, rest, ok := strings.Cut(inner, "\n")
	if !ok || !syntax.Known(first) {
		return "", inner
	}
	return first, rest
}

func commonMarkCodeFence(inner string) string {
	language, code := splitFenceLanguage(inner)
	return "```" + language + "\n" + code + "\n```"
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// Highlight paints a token once and lets the color run until the next
// token, but lipgloss closes every row and Hardwrap splits tokens across
// rows: re-open the running foreground at the start of each later row.
func reopenFgAcrossRows(rows string) string {
	split := strings.Split(rows, "\n")
	open := ""
	for i, row := range split {
		if open != "" {
			split[i] = open + row
		}
		for _, seq := range sgrRe.FindAllString(row, -1) {
			switch params := seq[2 : len(seq)-1]; {
			case params == "" || params == "0":
				open = ""
			case strings.HasPrefix(params, "38;"):
				open = seq
			}
		}
	}
	return strings.Join(split, "\n")
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
