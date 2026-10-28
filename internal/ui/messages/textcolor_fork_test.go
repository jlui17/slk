package messages

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
)

// Slack draws a context block's text muted throughout: the plain text
// after a bold span stays muted, as before it.
func TestContextBlockTextAfterBoldStaysMuted(t *testing.T) {
	styles.Apply("nord", config.Theme{})
	m := New([]MessageItem{{TS: "1.0", UserName: "bot", Blocks: []blockkit.Block{
		blockkit.ContextBlock{Elements: []blockkit.ContextElement{{Text: "*Owner* is here"}}},
	}}}, "general")
	m.buildCache(60, false)
	i := slices.IndexFunc(m.list.Entries(), func(e entrylist.Entry) bool { return e.Ref == 0 })
	row := ""
	for _, r := range m.list.Entries()[i].Lines {
		if strings.Contains(r, " is here") {
			row = r
		}
	}
	muted, primary := fgAt(fgANSIFor(styles.TextMuted)+"x", "x"), fgAt(FgANSI()+"x", "x")
	if got := fgAt(row, " is here"); got != muted {
		t.Errorf("text after the bold span is drawn in %q, want muted %q (primary is %q):\n%q", got, muted, primary, row)
	}
}

var fgParamsRe = regexp.MustCompile(`38;(?:2(?:;\d+){3}|5;\d+)`)

// fgAt is the truecolor or 256-color foreground in effect where sub
// starts in s, as the SGR parameters that set it; "" is the terminal's
// default.
func fgAt(s, sub string) string {
	fg := ""
	for _, seq := range sgrSeqRe.FindAllString(s[:strings.Index(s, sub)], -1) {
		switch params := seq[2 : len(seq)-1]; {
		case params == "" || params == "0" || params == "39":
			fg = ""
		case fgParamsRe.MatchString(params):
			fg = fgParamsRe.FindString(params)
		}
	}
	return fg
}
