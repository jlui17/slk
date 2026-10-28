package messages

import (
	"image/color"
	"io"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
)

// BlockkitContextFork is BlockkitContext with the fork's card state and
// text rendering: a card folds per ExpandedAttachments, and block text
// renders at its wrap width, keeps its emoji uploads (into
// emojiFlushes), highlights search terms and links mentions to
// profiles.
func BlockkitContextFork(msg MessageItem, rc *RenderContext, emojiFlushes *[]func(io.Writer) error) blockkit.Context {
	ctx := BlockkitContext(msg, rc)
	channelNames := rc.ChannelNames
	ctx.Card = CardContext(rc.ExpandedAttachments[msg.TS], channelNames)
	render := func(s string, un map[string]string, width int, textColor color.Color) string {
		return RenderSlackMarkdownWith(s, RenderSlackMarkdownOpts{
			UserNames:    un,
			ChannelNames: channelNames,
			UserGroups:   rc.UserGroups,
			PlaceCtx:     rc.Emoji.PlaceCtx,
			EmojiCells:   rc.Emoji.Cells,
			Customs:      rc.Emoji.Customs,
			EmojiFlushes: emojiFlushes,
			Width:        width,
			SearchTerms:  rc.SearchTerms,

			WorkspaceDomain: rc.WorkspaceDomain,
			TextColor:       textColor,
		})
	}
	ctx.RenderTextForWidth = func(s string, un map[string]string, width int) string {
		return render(s, un, width, nil)
	}
	ctx.RenderMutedText = func(s string, un map[string]string, width int) string {
		return render(s, un, width, styles.TextMuted)
	}
	return ctx
}
