package messages

func renderBlockquote(quoted string, width int) string {
	style := blockquoteStyle()
	return style.Render(WordWrap(quoted, width-style.GetHorizontalFrameSize()))
}
