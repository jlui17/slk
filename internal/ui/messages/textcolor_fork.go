package messages

import (
	"image/color"

	"github.com/gammons/slk/internal/ui/styles"
)

func (o RenderSlackMarkdownOpts) textFgANSI() string {
	if o.TextColor == nil {
		return BodyFgANSI()
	}
	return fgANSIFor(o.TextColor)
}

var (
	cachedBodyFgColor color.Color
	cachedBodyFgANSI  string
)

// BodyFgANSI is FgANSI for message body text: the TextBody role, which
// is TextPrimary unless the theme sets text_body.
func BodyFgANSI() string {
	fg := styles.TextBody
	if fg == cachedBodyFgColor && cachedBodyFgANSI != "" {
		return cachedBodyFgANSI
	}
	cachedBodyFgANSI = fgANSIFor(fg)
	cachedBodyFgColor = fg
	return cachedBodyFgANSI
}
