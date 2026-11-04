package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// A word bolded in part ("un" of "unbelievable") reaches the mrkdwn with
// a zero-width space beside its marker; y copies the text without it, as
// a paste into a shell or a search box would otherwise break the word.
func TestCopyMessage_PartlyBoldWordCopiesWithoutZeroWidthSpace(t *testing.T) {
	app := NewApp()
	var copied string
	app.SetClipboardWriter(func(text string) tea.Cmd {
		copied = text
		return nil
	})
	app.activeChannelID = "C123"
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{
		TS:       "1700000003.000400",
		UserName: "alice",
		Text:     "*un*believable",
		Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "un", Style: &slack.RichTextSectionTextStyle{Bold: true}},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "believable"},
			}},
		}}},
	}})

	cmd := app.handleNormalMode(tea.KeyPressMsg{Code: 'y', Text: "y"})
	if cmd == nil {
		t.Fatal("expected non-nil cmd from y key")
	}
	drainForCopiedMsg(cmd())
	if copied != "*un*believable" {
		t.Errorf("clipboard = %q, want %q", copied, "*un*believable")
	}
}
