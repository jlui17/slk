package messages

import (
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/styles"
)

// renderBodyText is styles.MessageText.Render(s), italic for a /me line
// as in Slack's own clients. For a style that sets only colours and
// text attributes, lipgloss puts every line between the style's opening
// and its reset and pads each short line to the widest with spaces in
// the style's whitespace colours, measuring every line four times over
// on the way. This measures each line once and takes both affixes from
// lipgloss itself (bodyTextAffixesFor). TestRenderBodyTextMatchesLipgloss
// pins the output to lipgloss's for every theme.
func renderBodyText(italic bool, s string) string {
	st := styles.MessageText
	if italic {
		st = st.Italic(true)
	}
	a := bodyTextAffixesFor(st, italic)
	// Lipgloss expands tabs and folds CR LF before styling.
	if !a.ok || strings.ContainsAny(s, "\t\r") {
		return st.Render(s)
	}
	lines := strings.Split(s, "\n")
	if len(lines) == 1 {
		return a.line.pre + s + a.line.suf
	}
	widths := make([]int, len(lines))
	widest := 0
	for i, l := range lines {
		widths[i] = ansi.StringWidth(l)
		widest = max(widest, widths[i])
	}
	var b strings.Builder
	b.Grow(len(s) + len(lines)*(len(a.line.pre)+len(a.line.suf)+len(a.pad.pre)+len(a.pad.suf)))
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(a.line.pre)
		b.WriteString(l)
		b.WriteString(a.line.suf)
		if pad := widest - widths[i]; pad > 0 {
			b.WriteString(a.pad.pre)
			for range pad {
				b.WriteByte(' ')
			}
			b.WriteString(a.pad.suf)
		}
	}
	return b.String()
}

// bodyTextAffixes is what lipgloss draws around each line of a body
// (line) and around a short line's padding (pad), for one theme
// version. ok is false when lipgloss's output did not have that shape,
// and renderBodyText then leaves the body to lipgloss.
type bodyTextAffixes struct {
	version   int64
	line, pad rowAffix
	ok        bool
}

// bodyTextAffixCache holds the affixes of the plain and the italic body
// style, rebuilt when the theme version moves on.
var bodyTextAffixCache struct {
	sync.Mutex
	byItalic [2]*bodyTextAffixes
}

func bodyTextAffixesFor(st lipgloss.Style, italic bool) bodyTextAffixes {
	i := 0
	if italic {
		i = 1
	}
	v := styles.Version()
	c := &bodyTextAffixCache
	c.Lock()
	defer c.Unlock()
	if a := c.byItalic[i]; a != nil && a.version == v {
		return *a
	}
	a := &bodyTextAffixes{version: v, line: rowAffixOf(st)}
	// "\n " renders as an empty line padded to one column, then the
	// space line: pre+suf, the padding's affixes around " ", newline.
	r := st.Render("\n ")
	first, _, _ := strings.Cut(r, "\n")
	if padded, found := strings.CutPrefix(first, a.line.pre+a.line.suf); found {
		if pre, suf, found := strings.Cut(padded, " "); found {
			a.pad, a.ok = rowAffix{pre: pre, suf: suf}, true
		}
	}
	c.byItalic[i] = a
	return *a
}
