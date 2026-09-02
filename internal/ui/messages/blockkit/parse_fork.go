package blockkit

import (
	"strconv"

	"github.com/slack-go/slack"
)

func parseTable(t *slack.TableBlock) TableBlock {
	out := TableBlock{Rows: make([][]string, 0, len(t.Rows))}
	for _, row := range t.Rows {
		cells := make([]string, len(row))
		for i, cell := range row {
			cells[i] = tableCellMrkdwn(cell)
		}
		out.Rows = append(out.Rows, cells)
	}
	return out
}

func tableCellMrkdwn(cell slack.TableCell) string {
	switch v := cell.(type) {
	case *slack.TableRichTextCell:
		return RichTextToMrkdwn(RichTextBlock{Elements: v.Elements})
	case *slack.TableRawTextCell:
		return v.Text
	case *slack.TableRawNumberCell:
		if v.Text != "" {
			return v.Text
		}
		return strconv.FormatFloat(v.Value, 'f', -1, 64)
	}
	return ""
}
