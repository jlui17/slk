package blockkit

import "github.com/gammons/slk/internal/core/blocks"

// TableBlock lives in core/blocks beside the Block interface it
// implements; the interface's method is unexported.
type TableBlock = blocks.TableBlock

func (ctx Context) renderText(s string, width int) string {
	if ctx.RenderTextForWidth != nil {
		return ctx.RenderTextForWidth(s, ctx.UserNames, width)
	}
	if ctx.RenderText != nil {
		return ctx.RenderText(s, ctx.UserNames)
	}
	return s
}
