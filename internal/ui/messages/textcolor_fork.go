package messages

func (o RenderSlackMarkdownOpts) textFgANSI() string {
	if o.TextColor == nil {
		return FgANSI()
	}
	return fgANSIFor(o.TextColor)
}
