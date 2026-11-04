package export

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// partlyBold is a message whose rich text bolds "un" of "unbelievable",
// which reaches the mrkdwn with a zero-width space beside the marker.
func partlyBold() messages.MessageItem {
	return messages.MessageItem{
		UserName: "alice", DateStr: "2026-05-18", Timestamp: "3:04 PM",
		Text: "*un*believable",
		Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "un", Style: &slack.RichTextSectionTextStyle{Bold: true}},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "believable"},
			}},
		}}},
	}
}

func TestThreadToMarkdown_PartlyBoldWordHasNoZeroWidthSpace(t *testing.T) {
	got := ThreadToMarkdown(partlyBold(), nil, nil, nil)
	if strings.Contains(got, blockkit.ZeroWidthSpace) || !strings.Contains(got, "**un**believable") {
		t.Errorf("want **un**believable with no U+200B, got:\n%q", got)
	}
}

func TestSnippet_PartlyBoldWordHasNoZeroWidthSpace(t *testing.T) {
	// The snippet flattens entities and keeps style markers.
	if got := snippet(partlyBold(), nil, nil); got != "*un*believable" {
		t.Errorf("snippet = %q, want %q", got, "*un*believable")
	}
}
