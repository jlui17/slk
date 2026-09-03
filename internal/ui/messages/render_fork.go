package messages

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var listItemRe = regexp.MustCompile(`^ *(• |\d+\. )`)

func renderBlockquote(quoted string, width int) string {
	style := blockquoteStyle()
	return style.Render(WordWrap(quoted, width-style.GetHorizontalFrameSize()))
}

func renderCodeBlock(inner string, width int) string {
	style := codeBlockStyle()
	inner = expandTabs(inner)
	if limit := width - style.GetHorizontalFrameSize(); limit > 0 {
		inner = ansi.Hardwrap(inner, limit, true)
	}
	return style.Render(inner)
}

func renderListItem(line string, opts RenderSlackMarkdownOpts) (string, bool) {
	head := listItemRe.FindString(line)
	if head == "" {
		return "", false
	}
	body := slackEntityDecoder.Replace(renderInlineFormattingWith(line[len(head):], opts))
	indent := strings.Repeat(" ", lipgloss.Width(head))
	body = WordWrap(body, opts.Width-len(indent))
	return head + strings.ReplaceAll(body, "\n", "\n"+indent), true
}

// lipgloss paints a tab as four cells but lipgloss.Width counts it as zero.
func expandTabs(s string) string {
	return strings.ReplaceAll(s, "\t", "    ")
}
