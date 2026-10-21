package blockkit

import (
	"strconv"
	"strings"

	"github.com/slack-go/slack"
)

// listLevel is one indent level of a run of lists.
type listLevel struct {
	drawn   bool
	ordered bool
	next    int // the number of the level's next ordered item, from 0
	textCol int // where the text of the level's last item starts
}

// listHeads returns, for each rich_text_list in elements, what goes
// before each of its items: the indent and the marker ("   ◦ ").
//
// Slack draws a run of adjacent lists as one nested list, as its own
// composer sends it: one rich_text_list per run of items at one indent.
// So an ordered list numbers on from the list before it at its indent
// when only deeper lists come between (Slack sends no offset for it),
// and restarts when anything else does; an explicit offset wins. A
// deeper list starts where its parent item's text starts. The markers
// cycle by indent as Slack's do: •, ◦, ▪ and 1., a., i.
func listHeads(elements []slack.RichTextElement) map[*slack.RichTextList][]string {
	heads := map[*slack.RichTextList][]string{}
	var run []listLevel
	for _, e := range elements {
		l, ok := e.(*slack.RichTextList)
		if !ok {
			run = nil
			continue
		}
		for len(run) <= l.Indent {
			run = append(run, listLevel{})
		}
		run = run[:l.Indent+1]
		lv := &run[l.Indent]
		ordered := l.Style == slack.RTEListOrdered
		if !lv.drawn || lv.ordered != ordered || l.Offset > 0 {
			*lv = listLevel{drawn: true, ordered: ordered, next: l.Offset}
		}
		pad := 2 * l.Indent
		if l.Indent > 0 && run[l.Indent-1].drawn {
			pad = run[l.Indent-1].textCol
		}
		for range l.Elements {
			marker := bulletMarker(l.Indent)
			if ordered {
				marker = orderedMarker(l.Indent, lv.next+1)
				lv.next++
			}
			head := strings.Repeat(" ", pad) + marker + " "
			lv.textCol = len([]rune(head))
			heads[l] = append(heads[l], head)
		}
	}
	return heads
}

func bulletMarker(indent int) string {
	return []string{"•", "◦", "▪"}[indent%3]
}

func orderedMarker(indent, n int) string {
	switch indent % 3 {
	case 1:
		return alphaNumeral(n) + "."
	case 2:
		return romanNumeral(n) + "."
	}
	return strconv.Itoa(n) + "."
}

// alphaNumeral counts a, b, ..., z, aa, ab, ... as CSS lower-alpha does.
func alphaNumeral(n int) string {
	s := ""
	for ; n > 0; n = (n - 1) / 26 {
		s = string(rune('a'+(n-1)%26)) + s
	}
	return s
}

func romanNumeral(n int) string {
	var b strings.Builder
	for _, d := range []struct {
		value  int
		digits string
	}{
		{1000, "m"}, {900, "cm"}, {500, "d"}, {400, "cd"}, {100, "c"}, {90, "xc"},
		{50, "l"}, {40, "xl"}, {10, "x"}, {9, "ix"}, {5, "v"}, {4, "iv"}, {1, "i"},
	} {
		for ; n >= d.value; n -= d.value {
			b.WriteString(d.digits)
		}
	}
	return b.String()
}
