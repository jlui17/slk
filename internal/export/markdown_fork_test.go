package export

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

func TestThreadToMarkdown_RichTextLiteralsExportLiterally(t *testing.T) {
	parent := messages.MessageItem{
		UserName: "alice", DateStr: "2026-05-18", Timestamp: "3:04 PM", Text: "lossy",
		Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "ping <@U1> then "},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "a < b", Style: &slack.RichTextSectionTextStyle{Code: true}},
			}},
		}}},
	}
	got := ThreadToMarkdown(parent, nil, map[string]string{"U1": "bob"}, nil)
	if !strings.Contains(got, "ping <@U1> then `a < b`") || strings.Contains(got, "@bob") {
		t.Errorf("literal rich_text characters reinterpreted on export:\n%s", got)
	}
}
