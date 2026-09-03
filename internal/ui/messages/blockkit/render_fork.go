package blockkit

import (
	"fmt"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
)

// RenderMessageBlocks renders a message's top-level blocks, skipping the
// one the host draws as the message body: the first rich_text block with
// content, the same rule as messages.MessageTextSource.
func RenderMessageBlocks(blocks []Block, ctx Context, width int) RenderResult {
	for i, b := range blocks {
		if rt, ok := b.(RichTextBlock); ok && RichTextToMrkdwn(rt) != "" {
			blocks = slices.Concat(blocks[:i], blocks[i+1:])
			break
		}
	}
	return Render(blocks, ctx, width)
}

func appendRichText(out *RenderResult, rt RichTextBlock, ctx Context, width int) {
	mrkdwn := RichTextToMrkdwn(rt)
	if mrkdwn == "" {
		return
	}
	for _, l := range renderTextLines(mrkdwn, ctx, width) {
		out.Lines = append(out.Lines, styles.MessageText.Render(l))
	}
}

const (
	tableLeft   = "│ "
	tableGutter = " │ "
	tableRight  = " │"
	// Below this many cells per column the host wrapper hard-breaks words
	// into fragments and a row grows many lines tall; a badge naming the
	// skipped table reads better than that.
	tableMinCellW = 6
)

func appendTable(out *RenderResult, t TableBlock, ctx Context, width int) {
	cols := 0
	for _, row := range t.Rows {
		if len(row) > cols {
			cols = len(row)
		}
	}
	if cols == 0 {
		return
	}

	rendered := make([][]string, len(t.Rows))
	natural := make([]int, cols)
	for r, row := range t.Rows {
		rendered[r] = make([]string, cols)
		for c, cell := range row {
			cell = ctx.renderText(cell, 0)
			rendered[r][c] = cell
			natural[c] = max(natural[c], lipgloss.Width(cell))
		}
	}
	chromeW := lipgloss.Width(tableLeft) + lipgloss.Width(tableRight) + lipgloss.Width(tableGutter)*(cols-1)
	if width-chromeW < tableMinCellW*cols {
		label := fmt.Sprintf("[table: %d columns, too wide for this pane]", cols)
		out.Lines = append(out.Lines, unsupportedStyle().Render(truncateToWidth(label, width)))
		return
	}
	colW := fitColumns(natural, width-chromeW)

	// mutedStyle, not dividerStyle: the Border color is too close to the
	// message background for the box to read as a table.
	line := mutedStyle()
	left, gutter, right := line.Render(tableLeft), line.Render(tableGutter), line.Render(tableRight)
	out.Lines = append(out.Lines, tableRule(colW, "┌─", "─┬─", "─┐"))
	parts := make([]string, cols)
	for r, row := range rendered {
		cell := styles.MessageText
		if r == 0 {
			cell = cell.Bold(true)
		}
		cells := make([][]string, cols)
		height := 0
		for c, text := range row {
			cells[c] = wrapCell(text, ctx, colW[c])
			for i, l := range cells[c] {
				cells[c][i] = cell.Render(l)
			}
			if len(cells[c]) > height {
				height = len(cells[c])
			}
		}
		for i := 0; i < height; i++ {
			for c := range parts {
				if i < len(cells[c]) {
					parts[c] = cells[c][i]
				} else {
					parts[c] = cell.Render(strings.Repeat(" ", colW[c]))
				}
			}
			out.Lines = append(out.Lines, left+strings.Join(parts, gutter)+right)
		}
		if r < len(rendered)-1 {
			out.Lines = append(out.Lines, tableRule(colW, "├─", "─┼─", "─┤"))
		}
	}
	out.Lines = append(out.Lines, tableRule(colW, "└─", "─┴─", "─┘"))
}

func tableRule(colW []int, left, joint, right string) string {
	spans := make([]string, len(colW))
	for c, w := range colW {
		spans[c] = strings.Repeat("─", w)
	}
	return mutedStyle().Render(left + strings.Join(spans, joint) + right)
}

// fitColumns takes width from the widest column first, so long prose
// cells wrap while short ones keep their natural width.
func fitColumns(natural []int, avail int) []int {
	colW := make([]int, len(natural))
	total := 0
	for i, w := range natural {
		if w < 1 {
			w = 1
		}
		colW[i] = w
		total += w
	}
	for total > avail {
		widest := 0
		for i, w := range colW {
			if w > colW[widest] {
				widest = i
			}
		}
		if colW[widest] <= 1 {
			break
		}
		colW[widest]--
		total--
	}
	return colW
}

func wrapCell(cell string, ctx Context, w int) []string {
	if ctx.WrapText != nil {
		cell = ctx.WrapText(cell, w)
	}
	lines := strings.Split(cell, "\n")
	for i, l := range lines {
		if lipgloss.Width(l) > w {
			l = ansi.Truncate(l, w, "…")
		}
		lines[i] = padRight(l, w)
	}
	return lines
}

// unsupportedStyle is the badge for content slk cannot draw: loud on
// purpose, so a reader knows the message is incomplete.
func unsupportedStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Bold(true).
		Foreground(styles.Background).
		Background(styles.Warning)
}
