package messages

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
)

// CodeBlock is one fenced block of a message body, as the c key and a
// click on the block's copy label put it on the clipboard.
type CodeBlock struct {
	Language string // the fence's tag; empty when it has none
	Code     string
}

// CodeBlocks returns msg's fenced code blocks in body order. It reads the
// text the panes render and splits it the way RenderSlackMarkdownWith
// does, so block N here is block N on screen. Tabs stay tabs: the
// renderer expands them only to paint.
func CodeBlocks(msg MessageItem) []CodeBlock {
	var blocks []CodeBlock
	for _, m := range codeBlockRe.FindAllStringSubmatch(MessageTextSource(msg), -1) {
		language, code := splitFenceLanguage(trimFenceNewlines(m[1]))
		blocks = append(blocks, CodeBlock{Language: language, Code: slackEntityDecoder.Replace(code)})
	}
	return blocks
}

// The same trim RenderSlackMarkdownWith applies inline (upstream code,
// left where it is): one line break per side, never the code's own
// indentation.
func trimFenceNewlines(inner string) string {
	if trimmed, ok := strings.CutPrefix(inner, "\r\n"); ok {
		inner = trimmed
	} else {
		inner = strings.TrimPrefix(inner, "\n")
	}
	if trimmed, ok := strings.CutSuffix(inner, "\r\n"); ok {
		return trimmed
	}
	return strings.TrimSuffix(inner, "\n")
}

// CodeBlockCopyLabel locates the clickable "copy" label on one fenced
// block's top border. Block indexes CodeBlocks. Row and the columns are
// cells of the render result once the caller has WordWrapped it to
// opts.Width; a pane moves them into its own entry's cells.
type CodeBlockCopyLabel struct {
	Block            int
	Row              int
	ColStart, ColEnd int // ColEnd exclusive
}

const copyLabelText = " copy "

func wearsCopyLabel(opts RenderSlackMarkdownOpts) bool {
	return opts.CodeBlockCopyLabels != nil && !opts.Preview
}

// The label sits one border cell in from the top right corner; a block
// too narrow to keep "╭─" in front of it goes without.
func copyLabelCols(blockWidth int) (start, end int, ok bool) {
	end = blockWidth - 2
	start = end - len(copyLabelText)
	return start, end, start >= 2
}

func withCopyLabel(block string) string {
	top, rest, _ := strings.Cut(block, "\n")
	width := lipgloss.Width(top)
	start, end, ok := copyLabelCols(width)
	if !ok {
		return block
	}
	// A filled chip like the reaction pills' "+": the app's look for
	// something a click acts on.
	label := lipgloss.NewStyle().Background(styles.Surface).Foreground(styles.Primary).Bold(true).Render(copyLabelText)
	return ansi.Cut(top, 0, start) + label + ansi.Cut(top, end, width) + "\n" + rest
}

// OffsetCodeBlockCopyLabels moves labels from body cells to the cells of
// the entry whose body starts rows down and cols in.
func OffsetCodeBlockCopyLabels(labels []CodeBlockCopyLabel, rows, cols int) []CodeBlockCopyLabel {
	for i := range labels {
		labels[i].Row += rows
		labels[i].ColStart += cols
		labels[i].ColEnd += cols
	}
	return labels
}

func CodeBlockCopyLabelAt(labels []CodeBlockCopyLabel, row, col int) (block int, ok bool) {
	for _, l := range labels {
		if row == l.Row && col >= l.ColStart && col < l.ColEnd {
			return l.Block, true
		}
	}
	return 0, false
}

func (m *Model) newBodyCopyLabels() *[]CodeBlockCopyLabel {
	m.bodyCopyLabels = nil
	return &m.bodyCopyLabels
}

// The body starts under the header row (and the broadcast row above
// it), right of the selection border and the avatar gutter.
func (m *Model) takeBodyCopyLabels(msg MessageItem, hasAvatar bool) []CodeBlockCopyLabel {
	rows, cols := 1, 1
	if msg.Subtype == "thread_broadcast" {
		rows++
	}
	if hasAvatar {
		cols += 5
	}
	labels := OffsetCodeBlockCopyLabels(m.bodyCopyLabels, rows, cols)
	m.bodyCopyLabels = nil
	return labels
}

// CodeBlockAt returns the code of the fenced block whose copy label is
// drawn at pane-local (y, x), the frame ClickAt takes.
func (m *Model) CodeBlockAt(y, x int) (code string, ok bool) {
	if y < m.chromeHeight {
		return "", false
	}
	line := y - m.chromeHeight + m.yOffset
	for i, e := range m.cache {
		block, hit := CodeBlockCopyLabelAt(e.codeBlockCopyLabels, line-m.entryOffsets[i], x)
		if !hit {
			continue
		}
		blocks := CodeBlocks(m.messages[e.msgIdx])
		if block >= len(blocks) {
			return "", false
		}
		return blocks[block].Code, true
	}
	return "", false
}
