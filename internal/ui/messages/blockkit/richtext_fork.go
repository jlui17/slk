package blockkit

import (
	"strings"

	"github.com/gammons/slk/internal/ui/syntax"
)

// A rich_text text element carries the characters the author typed,
// unescaped.
var literalTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeLiteralText(text string) string {
	return literalTextEscaper.Replace(text)
}

// Slack tags a preformatted block with the language its picker chose;
// only languages the renderer can highlight ride on the fence, so a
// tag it can't use never shows up as a first line of code.
func withFenceLanguage(fence, language string) string {
	if language == "" || !syntax.Known(language) {
		return fence
	}
	return strings.Replace(fence, "```\n", "```"+language+"\n", 1)
}
