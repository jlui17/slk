package blockkit

import (
	"encoding/json"
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

// fillButtonFields gives each button of elems, parsed from a's elements
// in order, its ButtonFields.
func fillButtonFields(elems []ActionElement, a *slack.ActionBlock) {
	for i, e := range a.Elements.ElementSet {
		if b, ok := e.(*slack.ButtonBlockElement); ok {
			elems[i].ButtonFields = buttonFields(b, a.BlockID)
		}
	}
}

// withAccessoryButtonFields returns acc, parsed from s's accessory, with
// its ButtonFields when the accessory is a button.
func withAccessoryButtonFields(acc AccessoryElement, s *slack.SectionBlock) AccessoryElement {
	label, ok := acc.(LabelAccessory)
	if !ok || s.Accessory.ButtonElement == nil {
		return acc
	}
	label.ButtonFields = buttonFields(s.Accessory.ButtonElement, s.BlockID)
	return label
}

func buttonFields(b *slack.ButtonBlockElement, blockID string) ButtonFields {
	return ButtonFields{ActionID: b.ActionID, BlockID: blockID, Value: b.Value, URL: b.URL}
}

// ParseJSON parses one block from its JSON, as a Home view carries it.
// A block that doesn't parse is an UnknownBlock of typ.
func ParseJSON(raw []byte, typ string) Block {
	var set slack.Blocks
	if err := json.Unmarshal([]byte("["+string(raw)+"]"), &set); err != nil || len(set.BlockSet) != 1 {
		return UnknownBlock{Type: typ}
	}
	return Parse(set)[0]
}
