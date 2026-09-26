package messages

import "strings"

type CopyableKind int

const (
	CopyableCodeBlock CopyableKind = iota
	CopyableLink
)

// Copyable is one thing the c key can copy out of a message: a fenced
// code block or a link.
type Copyable struct {
	Kind CopyableKind
	Text string // what lands on the clipboard

	CodeBlock CodeBlock // set when Kind is CopyableCodeBlock
	Link      Link      // set when Kind is CopyableLink
}

// Copyables returns msg's fenced code blocks and links in body order.
// Both come from the text the panes render (MessageTextSource), not
// msg.Text, so one set of positions orders them. A link inside a fence
// is not an item of its own: copying the block carries it. Links are
// deduplicated by URL as ExtractLinks does, first occurrence outside a
// fence winning.
func Copyables(msg MessageItem) []Copyable {
	text := MessageTextSource(msg)
	var items []Copyable
	seen := map[string]bool{}
	addLinks := func(outsideFences string) {
		for _, l := range ExtractLinks(outsideFences) {
			if seen[l.URL] {
				continue
			}
			seen[l.URL] = true
			// Slack sends & inside a URL as &amp;.
			items = append(items, Copyable{Kind: CopyableLink, Text: strings.ReplaceAll(l.URL, "&amp;", "&"), Link: l})
		}
	}
	end := 0
	for _, m := range codeBlockRe.FindAllStringSubmatchIndex(text, -1) {
		addLinks(text[end:m[0]])
		block := codeBlockOfFence(text[m[2]:m[3]])
		items = append(items, Copyable{Kind: CopyableCodeBlock, Text: block.Code, CodeBlock: block})
		end = m[1]
	}
	addLinks(text[end:])
	return items
}
