package blockkit

import "strings"

// A rich_text text element carries the characters the author typed,
// unescaped.
var literalTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeLiteralText(text string) string {
	return literalTextEscaper.Replace(text)
}
