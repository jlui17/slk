package blockkit

import (
	"regexp"
	"strings"
)

// A rich_text text element carries the characters the author typed,
// unescaped.
var literalTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func escapeLiteralText(text string) string {
	return literalTextEscaper.Replace(text)
}

var fenceLanguageRe = regexp.MustCompile(`^[A-Za-z0-9_+#.-]+$`)

// The tag rides as <lang> right after the fence: Slack escapes < in code
// text, so nothing delivered can collide with it. Any fence-safe language
// rides along; the renderer decides what it can highlight.
func withFenceLanguage(fence, language string) string {
	if !fenceLanguageRe.MatchString(language) {
		return fence
	}
	return "```<" + language + ">" + strings.TrimPrefix(fence, "```")
}
