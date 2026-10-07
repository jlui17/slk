// Package blockkittest holds helpers for tests that render Block Kit
// content through a host pane.
package blockkittest

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// Paragraph is a rich_text block holding one plain text run.
func Paragraph(text string) blockkit.RichTextBlock {
	return blockkit.RichTextBlock{Elements: []slack.RichTextElement{
		&slack.RichTextSection{
			Type: slack.RTESection,
			Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: text},
			},
		},
	}}
}

// RunesWithoutBackground replays a line's SGR sequences and collects the
// runes, spaces included, printed while no 48;… background parameter is
// active: a bare space shows the terminal background just as text does.
func RunesWithoutBackground(line string) string {
	var bare strings.Builder
	bgSet := false
	for i := 0; i < len(line); {
		if strings.HasPrefix(line[i:], "\x1b[") {
			end := strings.IndexByte(line[i:], 'm')
			if end < 0 {
				break
			}
			params := strings.Split(line[i+2:i+end], ";")
			for j := 0; j < len(params); j++ {
				switch params[j] {
				case "", "0", "49":
					bgSet = false
				case "48":
					bgSet = true
					if j+1 < len(params) && params[j+1] == "2" {
						j += 4
					} else if j+1 < len(params) && params[j+1] == "5" {
						j += 2
					}
				}
			}
			i += end + 1
			continue
		}
		rn, size := utf8.DecodeRuneInString(line[i:])
		if !bgSet {
			bare.WriteRune(rn)
		}
		i += size
	}
	return bare.String()
}

// FromJSON parses a message's "blocks" array as it arrives from Slack, so
// a fixture can hold elements slack-go has no type for.
func FromJSON(blocksJSON string) []blockkit.Block {
	var blocks slack.Blocks
	if err := json.Unmarshal([]byte(blocksJSON), &blocks); err != nil {
		panic(err)
	}
	return blockkit.Parse(blocks)
}

// ColonyEphemeral is the blocks of the ephemeral the Colony app posts
// after an annotation: the section links the annotated message, and the
// actions block's one button links the review to complete.
func ColonyEphemeral(messageURL, reviewURL string) []blockkit.Block {
	return FromJSON(`[
		{"type":"section","block_id":"sent","text":{"type":"mrkdwn","text":"*Your annotation was sent.*\nFor <` + messageURL + `|this message>. Label: *negative*.\n> note text"}},
		{"type":"context","elements":[{"type":"mrkdwn","text":"A judge is reading the thread now. It tags you in the review when it has a verdict."}]},
		{"type":"actions","block_id":"review","elements":[
			{"type":"button","action_id":"open_review","text":{"type":"plain_text","text":"Complete review"},"style":"primary","url":"` + reviewURL + `"}]}]`)
}
