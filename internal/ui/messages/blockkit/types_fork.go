package blockkit

// TableBlock is the Slack `table` block. Slack draws the first row as the
// header.
type TableBlock struct {
	Rows [][]string
}

func (TableBlock) blockType() string { return "table" }

func (ctx Context) renderText(s string, width int) string {
	if ctx.RenderTextForWidth != nil {
		return ctx.RenderTextForWidth(s, ctx.UserNames, width)
	}
	if ctx.RenderText != nil {
		return ctx.RenderText(s, ctx.UserNames)
	}
	return s
}
