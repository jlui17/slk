package blockkit

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/slackurl"
)

func richTextCell(text string, code bool) *slack.TableRichTextCell {
	el := &slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: text}
	if code {
		el.Style = &slack.RichTextSectionTextStyle{Code: true}
	}
	return slack.NewTableRichTextCell(
		&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{el}},
	)
}

func TestParseTableCellsBecomeMrkdwn(t *testing.T) {
	in := slack.Blocks{BlockSet: []slack.Block{
		&slack.TableBlock{
			Type: slack.MBTTable,
			Rows: [][]slack.TableCell{
				{richTextCell("Step", false), richTextCell("Adapter does", false)},
				{richTextCell("Auth()", true), nil},
				{slack.NewTableRawTextCell("~1,980 lines"), slack.NewTableRawNumberCell(255)},
				{slack.NewTableRawNumberCell(1980).WithText("~1,980"), slack.NewTableRawTextCell("")},
			},
		},
	}}
	got := Parse(in)
	if len(got) != 1 {
		t.Fatalf("Parse produced %d blocks, want 1", len(got))
	}
	tb, ok := got[0].(TableBlock)
	if !ok {
		t.Fatalf("Parse produced %T, want TableBlock", got[0])
	}
	want := [][]string{
		{"Step", "Adapter does"},
		{"`Auth()`", ""},
		{"~1,980 lines", "255"},
		{"~1,980", ""},
	}
	if !reflect.DeepEqual(tb.Rows, want) {
		t.Errorf("Rows = %q, want %q", tb.Rows, want)
	}
}

// The wire shape of a Slack message unfurl. Slack escapes nothing in
// from_url; the ts is the linked message's own, which the header reads
// from the permalink instead.
func TestParseAttachmentKeepsTheLinkedMessage(t *testing.T) {
	var a slack.Attachment
	if err := json.Unmarshal([]byte(`{
		"from_url": "https://example.slack.com/archives/C0000000001/p1736000000000200?thread_ts=1736000000.000100&cid=C0000000001",
		"ts": "1736000000.000200",
		"author_id": "U0000000001",
		"author_name": "Claude",
		"author_subname": "Claude",
		"channel_id": "C0000000001",
		"is_msg_unfurl": true,
		"is_reply_unfurl": true,
		"text": "hi",
		"footer": "Thread in Slack Conversation"
	}`), &a); err != nil {
		t.Fatal(err)
	}
	link, ok := LinkedMessage(parseAttachment(a))
	want := slackurl.Permalink{Subdomain: "example", ChannelID: "C0000000001", MessageTS: "1736000000.000200", ThreadTS: "1736000000.000100"}
	if !ok || link != want {
		t.Errorf("LinkedMessage = %+v, %v; want %+v", link, ok, want)
	}
	if _, ok := LinkedMessage(LegacyAttachment{FromURL: "https://example.slack.com/archives/C0000000001/p1736000000000200"}); ok {
		t.Error("an attachment with no author is not a linked message")
	}
}
