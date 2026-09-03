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
	inner = expandTabs(slackEntityDecoder.Replace(inner))
	return style.Render(ansi.Hardwrap(inner, width-style.GetHorizontalFrameSize(), true))
}

func renderListItem(line string, opts RenderSlackMarkdownOpts) (string, bool) {
	head := listItemRe.FindString(line)
	if head == "" {
		return "", false
	}
	body := slackEntityDecoder.Replace(renderInlineFormattingWith(line[len(head):], opts))
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
