package messages

import (
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// Slack escapes and reorders a permalink's query between the text and
// the card's from_url, so the two meet on what they point at.
type permalinkTarget struct {
	channel ids.ChannelID
	ts      ids.MessageTS
}

// PermalinkChips holds, for each linked-message card a message carries,
// the short label that message's permalink wears in the body.
type PermalinkChips map[permalinkTarget]string

func PermalinkChipsOf(msg MessageItem, channelNames map[string]string) PermalinkChips {
	var chips PermalinkChips
	for _, a := range msg.LegacyAttachments {
		link, ok := blockkit.LinkedMessage(a)
		if !ok {
			continue
		}
		label := "↳ " + a.AuthorName
		if channel := blockkit.ChannelLabel(link, channelNames); channel != "" {
			label += " in " + channel
		}
		if chips == nil {
			chips = PermalinkChips{}
		}
		chips[permalinkTarget{link.ChannelID, link.MessageTS}] = label
	}
	return chips
}

// label is what a link shows. Only a link drawn as its own URL becomes
// a chip: a label the author wrote stays.
func (c PermalinkChips) label(url, visible string) string {
	if len(c) == 0 || visible != url {
		return visible
	}
	link, ok := slackurl.Parse(url)
	if !ok {
		return visible
	}
	if chip, ok := c[permalinkTarget{link.ChannelID, link.MessageTS}]; ok {
		return chip
	}
	return visible
}
