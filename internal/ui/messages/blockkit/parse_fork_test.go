package blockkit

import (
	"reflect"
	"testing"

	"github.com/slack-go/slack"
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
