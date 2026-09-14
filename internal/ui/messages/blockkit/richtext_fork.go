package blockkit

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/slack-go/slack"
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

// Slack links a message inline as a message_mention element, which
// slack-go v0.29 does not model, so it arrives with only its raw JSON.
// Slack's own text fallback spells it as a bare <url>.
func unknownInlineToMrkdwn(e *slack.RichTextSectionUnknownElement) string {
	if e.Type != "message_mention" {
		return ""
	}
	var mention struct {
		URL string `json:"url"`
	}
	if json.Unmarshal([]byte(e.Raw), &mention) != nil || mention.URL == "" {
		return ""
	}
	return "<" + mention.URL + ">"
}
