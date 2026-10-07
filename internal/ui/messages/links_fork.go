package messages

import (
	"slices"
	"strings"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// MessageLinks returns the links msg offers to o and O, one per URL: the
// links of the text, then the links of the blocks, in reading order. The
// body's rich_text block gives the links of the text what the text does
// not carry, their list item and their link text, and adds the links a
// bot's short text fallback leaves out. Cards and files are not read.
func MessageLinks(msg MessageItem) []Link {
	var links []Link
	at := map[string]int{}
	body, below := blockLinks(msg.Blocks)
	for _, l := range slices.Concat(ExtractLinks(msg.Text), body, below) {
		if i, ok := at[linkKey(l.URL)]; ok {
			links[i].learn(l)
			continue
		}
		at[linkKey(l.URL)] = len(links)
		links = append(links, l)
	}
	return links
}

// linkKey is the URL as the dedup compares it: the message text spells
// an & in a URL as &amp;, a rich_text link element as &.
func linkKey(url string) string { return strings.ReplaceAll(url, "&amp;", "&") }

// learn gives a link that was collected before another copy of it what
// that copy knows and it lacks: its Context and its link text. The link
// keeps its first place.
func (l *Link) learn(again Link) {
	if l.Context == "" {
		l.Context = again.Context
	}
	if l.Label == "" {
		l.Label = again.Label
	}
}

// blockLinks returns the links of the rich_text block the host draws as
// the body, and of the blocks RenderMessageBlocks draws below it. A link
// carries what it sits with as its Context, as mrkdwn, for the app to
// name the row with the names only it can resolve: the first cell of a
// table row, a list item without its links. A link button is a link
// named by its text.
func blockLinks(blocks []blockkit.Block) (body, below []Link) {
	bodySeen := false
	for _, b := range blocks {
		switch v := b.(type) {
		case blockkit.RichTextBlock:
			links := &below
			if !bodySeen && blockkit.RichTextToMrkdwn(v) != "" {
				links, bodySeen = &body, true
			}
			for _, l := range blockkit.RichTextLinks(v) {
				*links = append(*links, Link{URL: l.URL, Label: l.Text, Context: l.ListItem})
			}
		case blockkit.SectionBlock:
			below = append(below, ExtractLinks(v.Text)...)
			for _, f := range v.Fields {
				below = append(below, ExtractLinks(f)...)
			}
			if acc, ok := v.Accessory.(blockkit.LabelAccessory); ok && acc.URL != "" {
				below = append(below, Link{URL: acc.URL, Label: acc.Label})
			}
		case blockkit.ActionsBlock:
			for _, e := range v.Elements {
				if e.URL != "" {
					below = append(below, Link{URL: e.URL, Label: e.Label})
				}
			}
		case blockkit.ContextBlock:
			for _, e := range v.Elements {
				below = append(below, ExtractLinks(e.Text)...)
			}
		case blockkit.TableBlock:
			for _, row := range v.Rows {
				for c, cell := range row {
					for _, l := range ExtractLinks(cell) {
						if c > 0 {
							l.Context = row[0]
						}
						below = append(below, l)
					}
				}
			}
		}
	}
	return body, below
}

// InteractHint is the line drawn under a message's interactive Block
// Kit elements. When every one of them is a link button, o opens them,
// so the hint says that; a control in a card is never one o offers.
func InteractHint(msg MessageItem) string {
	all, links := controls(msg.Blocks)
	for _, a := range msg.LegacyAttachments {
		n, _ := controls(a.Blocks)
		all += n
	}
	if all > 0 && all == links {
		return "o to open"
	}
	return "↗ open in Slack to interact"
}

// controls counts the controls blocks draw, a section's non-image
// accessory or an actions block's element, and the link buttons among
// them.
func controls(blocks []blockkit.Block) (all, links int) {
	count := func(kind, url string) {
		all++
		if kind == "button" && url != "" {
			links++
		}
	}
	for _, b := range blocks {
		switch v := b.(type) {
		case blockkit.SectionBlock:
			if acc, ok := v.Accessory.(blockkit.LabelAccessory); ok {
				count(acc.Kind, acc.URL)
			}
		case blockkit.ActionsBlock:
			for _, e := range v.Elements {
				count(e.Kind, e.URL)
			}
		}
	}
	return all, links
}
