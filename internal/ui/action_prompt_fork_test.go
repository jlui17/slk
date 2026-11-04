package ui

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// The button dialog draws the message as plain text, so a word bolded in
// part reaches it whole, without the zero-width space the mrkdwn carries.
func TestActionBody_PartlyBoldWordHasNoZeroWidthSpace(t *testing.T) {
	app := NewApp()
	body := app.actionBody(messages.MessageItem{
		TS:   "1700000003.000400",
		Text: "*un*believable",
		Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "un", Style: &slack.RichTextSectionTextStyle{Bold: true}},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "believable"},
			}},
		}}},
	})
	if strings.Contains(body, blockkit.ZeroWidthSpace) || body != "*un*believable" {
		t.Errorf("actionBody = %q, want %q", body, "*un*believable")
	}
}
