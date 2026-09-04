package messages

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
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
	body = strings.Join(reopenSGRAcrossRows(strings.Split(body, "\n")), "\n")
	return style.Render(body)
}

type heldCodeBlocks []string

const codeBlockMarkerPrefix = "\x00CB"

func codeBlockMarker(i int) string {
	return fmt.Sprintf("%s%d\x00", codeBlockMarkerPrefix, i)
}

func isCodeBlockMarker(line string) bool {
	return strings.HasPrefix(line, codeBlockMarkerPrefix) && strings.HasSuffix(line, "\x00")
}

func (h *heldCodeBlocks) hold(rendered string) string {
	*h = append(*h, rendered)
	return codeBlockMarker(len(*h) - 1)
}

func (h heldCodeBlocks) restore(s string) string {
	for i, block := range h {
		s = strings.Replace(s, codeBlockMarker(i), block, 1)
	}
	return s
}

// A held block sits exactly one blank row from whatever surrounds it,
// whatever spacing the author or the block-to-mrkdwn join produced.
func (h heldCodeBlocks) tighten(text string) string {
	if len(h) == 0 {
		return text
	}
	var out []string
	afterBlock := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case isCodeBlockMarker(line):
			for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
				out = out[:len(out)-1]
			}
			if len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, line)
			afterBlock = true
			continue
		case afterBlock && strings.TrimSpace(line) == "":
			continue
		case afterBlock:
			out = append(out, "")
		}
		out = append(out, line)
		afterBlock = false
	}
	return strings.Join(out, "\n")
}

func renderCodeBlock(inner string, opts RenderSlackMarkdownOpts, hl searchHighlighter) string {
	language, code := splitFenceLanguage(inner)
	code = expandTabs(slackEntityDecoder.Replace(code))
	if opts.Preview {
		return codeBlockStyle().Render(hl.highlight(code))
	}
	style := codeBlockStyle().Inherit(styles.UnfocusedBorder).Padding(1, 1)
	lexer := syntax.Lexer(language)
	if lexer != nil {
		code = highlightCode(code, lexer)
	}
	lines := strings.Split(code, "\n")
	digits := 0
	if lexer != nil {
		digits = len(fmt.Sprint(len(lines)))
	}
	limit := opts.Width - style.GetHorizontalFrameSize()
	if digits > 0 {
		limit -= digits + 2
	}
	var rows, gutters []string
	for n, line := range lines {
		for j, row := range strings.Split(ansi.Hardwrap(line, limit, true), "\n") {
			rows = append(rows, row)
			gutters = append(gutters, gutter(digits, n+1, j == 0))
		}
	}
	surface := bgANSIFor(styles.Surface)
	for i, row := range reopenSGRAcrossRows(rows) {
		rows[i] = gutters[i] + ReapplyBgAfterResets(hl.highlight(row), surface)
	}
	body := strings.Join(rows, "\n")
	if lexer != nil {
		style = style.PaddingTop(0)
		body = fgANSIFor(styles.TextMuted) + lexer.Config().Name + "\n\n" + body
	}
	if limit > 0 {
		style = style.Width(opts.Width)
	}
	return style.Render(body)
}

// The gutter is two cells wider than its digits and restores the text
// color after them, so the row's own color run resumes untouched; a
// wrapped continuation row gets a blank gutter.
func gutter(digits, n int, lineStart bool) string {
	switch {
	case digits == 0:
		return ""
	case lineStart:
		return fmt.Sprintf("%s%*d  %s", fgANSIFor(styles.TextMuted), digits, n, fgANSIFor(styles.TextPrimary))
	default:
		return strings.Repeat(" ", digits+2)
	}
}

var paintedCode = struct {
	sync.Mutex
	version int64
	byKey   map[string]string
}{byKey: map[string]string{}}

// Tokenising costs milliseconds per tagged block and the messages pane
// re-renders every message on each cache rebuild, so painted text is kept
// per lexer, code, and theme version.
func highlightCode(code string, lexer chroma.Lexer) string {
	key := lexer.Config().Name + "\x00" + code
	paintedCode.Lock()
	defer paintedCode.Unlock()
	if paintedCode.version != styles.Version() || len(paintedCode.byKey) > 512 {
		paintedCode.byKey = map[string]string{}
		paintedCode.version = styles.Version()
	}
	if painted, ok := paintedCode.byKey[key]; ok {
		return painted
	}
	painted := paintSpans(syntax.Highlight(code, lexer))
	paintedCode.byKey[key] = painted
	return painted
}

// One foreground change per color run and no resets, so the box's
// background and lipgloss's row framing stay in force.
func paintSpans(spans []syntax.Span) string {
	var b strings.Builder
	current := ""
	for _, s := range spans {
		if fg := fgANSIFor(s.Color); fg != current {
			b.WriteString(fg)
			current = fg
		}
		b.WriteString(s.Text)
	}
	return b.String()
}

var fenceTagRe = regexp.MustCompile(`^<([^<>\s]+)>\n?`)

// Slack escapes < in delivered code, so before entity decoding a raw <lang>
// after the fence can only be the tag blockkit put there.
func splitFenceLanguage(inner string) (language, code string) {
	m := fenceTagRe.FindStringSubmatch(inner)
	if m == nil {
		return "", inner
	}
	return m[1], strings.TrimLeft(inner[len(m[0]):], "\n")
}

func commonMarkCodeFence(inner string) string {
	language, code := splitFenceLanguage(inner)
	return "```" + language + "\n" + slackEntityDecoder.Replace(code) + "\n```"
}

var sgrRe = regexp.MustCompile("\x1b\\[[0-9;]*m")

// lipgloss closes every row, and wrapping splits styled runs across rows:
// re-open at each later row's start whatever SGR the row before left open.
// A sequence seen again moves to the end, so the list stays bounded and
// the latest foreground wins; a closer (39, 49, 22...) is not carried and
// retires the opens it cancels.
func reopenSGRAcrossRows(rows []string) []string {
	var open []string
	for i, row := range rows {
		if len(open) > 0 {
			rows[i] = strings.Join(open, "") + row
		}
		for _, seq := range sgrRe.FindAllString(row, -1) {
			params := seq[2 : len(seq)-1]
			switch closes := sgrClosers[params]; {
			case params == "" || params == "0":
				open = open[:0]
			case closes != "":
				open = slices.DeleteFunc(open, func(s string) bool { return strings.Contains(sgrSets(s), closes) })
			default:
				open = append(slices.DeleteFunc(open, func(s string) bool { return s == seq }), seq)
			}
		}
	}
	return rows
}

var sgrClosers = map[string]string{"39": "f", "49": "b", "22": "B", "23": "i", "24": "u", "27": "r", "29": "s"}

// sgrSets names the attributes an SGR sequence sets, one letter each:
// f foreground, b background, B bold/faint, i italic, u underline,
// r reverse, s strike.
func sgrSets(seq string) string {
	var kinds strings.Builder
	params := strings.Split(seq[2:len(seq)-1], ";")
	for i := 0; i < len(params); i++ {
		switch p := params[i]; {
		case p == "38" || p == "48":
			if p == "38" {
				kinds.WriteByte('f')
			} else {
				kinds.WriteByte('b')
			}
			if i+1 < len(params) && params[i+1] == "2" {
				i += 4
			} else {
				i += 2
			}
		case p >= "30" && p <= "37" || p >= "90" && p <= "97":
			kinds.WriteByte('f')
		case p >= "40" && p <= "47" || p >= "100" && p <= "107":
			kinds.WriteByte('b')
		case p == "1" || p == "2":
			kinds.WriteByte('B')
		case p == "3":
			kinds.WriteByte('i')
		case p == "4":
			kinds.WriteByte('u')
		case p == "7":
			kinds.WriteByte('r')
		case p == "9":
			kinds.WriteByte('s')
		}
	}
	return kinds.String()
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
