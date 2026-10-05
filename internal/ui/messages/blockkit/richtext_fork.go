package blockkit

import (
	"encoding/json"
	"regexp"
	"strconv"
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

// Slack does not nest a list in a rich_text_quote: the lists of a quote
// follow it as siblings that carry border 1. They join the quote's "> "
// lines, so the renderer draws one quote. A bordered rich_text_preformatted
// is left as it is: a "> " on each fence line would stop it being a fence.
func withQuoteBorder(mrkdwn string, border int) string {
	if border == 0 {
		return mrkdwn
	}
	return "> " + strings.ReplaceAll(mrkdwn, "\n", "\n> ")
}

// Slack links a message inline as a message_mention element, which
// slack-go does not model, so it arrives with only its raw JSON.
// Slack's own text fallback spells it as a bare <url>.
func unknownInlineToMrkdwn(e *slack.RichTextSectionUnknownElement) string {
	if url := messageMentionURL(e); url != "" {
		return linkToMrkdwn(url, "")
	}
	return ""
}

func messageMentionURL(e *slack.RichTextSectionUnknownElement) string {
	if e.Type != "message_mention" {
		return ""
	}
	var mention struct {
		URL string `json:"url"`
	}
	json.Unmarshal([]byte(e.Raw), &mention)
	return mention.URL
}

// RichTextLink is one link element of a rich_text block.
type RichTextLink struct {
	URL  string
	Text string // empty when the element has none of its own
	// ListItem is the list item the link sits in, as mrkdwn: the item
	// without its links, behind its number when the list is ordered
	// ("7. <@U1>", or "7." when the item is nothing but links). Empty
	// outside a list, and in a bullet item that is nothing but links.
	ListItem string
}

// RichTextLinks returns the link and message_mention elements of rt in
// reading order. A link in a code block is not one of them: it has no
// text and no list item, which is all a caller gets here that the
// mrkdwn does not already give.
func RichTextLinks(rt RichTextBlock) []RichTextLink {
	var links []RichTextLink
	for _, e := range rt.Elements {
		switch v := e.(type) {
		case *slack.RichTextSection:
			links = append(links, sectionLinks(v.Elements, "")...)
		case *slack.RichTextQuote:
			links = append(links, sectionLinks(v.Elements, "")...)
		case *slack.RichTextList:
			for i, child := range v.Elements {
				if sec, ok := child.(*slack.RichTextSection); ok {
					links = append(links, sectionLinks(sec.Elements, listItemWithoutLinks(v, i, sec.Elements))...)
				}
			}
		}
	}
	return links
}

func sectionLinks(elements []slack.RichTextSectionElement, listItem string) []RichTextLink {
	var links []RichTextLink
	for _, e := range elements {
		if url, text := inlineLink(e); url != "" {
			links = append(links, RichTextLink{URL: url, Text: text, ListItem: listItem})
		}
	}
	return links
}

// inlineLink returns e's URL and its text, "" when it has none of its
// own, or "", "" when e is not a link.
func inlineLink(e slack.RichTextSectionElement) (url, text string) {
	switch v := e.(type) {
	case *slack.RichTextSectionLinkElement:
		if v.Text != v.URL {
			text = v.Text
		}
		return v.URL, text
	case *slack.RichTextSectionUnknownElement:
		return messageMentionURL(v), ""
	}
	return "", ""
}

// listItemEdges is what joins an item's text to a link at its start or
// end ("<person> - <link>", "<link>: <text>").
const listItemEdges = " \n-–—:,"

// listItemWithoutLinks is item i of l as RichTextLink.ListItem wants it.
// The number is the one listToMrkdwn draws. Where a link is taken from
// the start or the end of the item, the typed text beside it loses its
// listItemEdges and the bracket the link leaves unmatched; an edge that
// is the item's own text, or an :emoji:, stays as it is. A bracket pair
// a link leaves empty goes too.
func listItemWithoutLinks(l *slack.RichTextList, i int, elements []slack.RichTextSectionElement) string {
	item := ""
	linkTaken := false // since the last element kept
	lastTextAt := -1   // where the last element kept starts in item, when it is typed text
	for _, e := range elements {
		if url, _ := inlineLink(e); url != "" {
			linkTaken = true
			continue
		}
		part := inlineToMrkdwn(e)
		_, isText := e.(*slack.RichTextSectionTextElement)
		if linkTaken && isText {
			switch {
			case item == "":
				part = strings.TrimLeft(part, listItemEdges+")]")
			case lastTextAt >= 0 && (strings.HasSuffix(item, "(") && strings.HasPrefix(part, ")") ||
				strings.HasSuffix(item, "[") && strings.HasPrefix(part, "]")):
				item, part = item[:len(item)-1], part[1:]
			}
		}
		linkTaken, lastTextAt = false, -1
		if isText {
			lastTextAt = len(item)
		}
		item += part
	}
	if linkTaken && lastTextAt >= 0 {
		item = item[:lastTextAt] + strings.TrimRight(item[lastTextAt:], listItemEdges+"([")
	}
	item = strings.TrimSpace(item)
	if l.Style == slack.RTEListOrdered {
		item = strings.TrimSpace(strconv.Itoa(l.Offset+i+1) + ". " + item)
	}
	return item
}
