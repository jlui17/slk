package messages

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	"github.com/gammons/slk/internal/ui/styles"
)

// wrapRowsLipgloss is WrapRows as it was before rows were assembled
// directly: a lipgloss render for every fill and border pass. The tests
// below hold WrapRows to it byte for byte.
func wrapRowsLipgloss(msg MessageItem, r RenderedMessage, width int, s WrapStyles) WrappedRows {
	rendered := r.Content
	renderedSel := SelectedHeader(rendered, r.Header, msg.TS, msg.Timestamp, r.HeaderBudget)
	filledNormal := s.fill.Width(width - 1).Render(rendered)
	var shortSrc string
	if renderedSel != rendered {
		shortSrc = rendered
	}
	return WrappedRows{
		Lines:    strings.Split(s.invis.Render(filledNormal), "\n"),
		Selected: selectedVariantLipgloss(s, renderedSel, width),
		Plain:    entrylist.PlainLines(filledNormal),
		ShortSrc: shortSrc,
	}
}

func selectedVariantLipgloss(s WrapStyles, content string, width int) []string {
	fill := lipgloss.NewStyle().Background(styles.SelectionTintColor(s.focused)).Width(width - 1)
	tinted := ReapplyBgAfterResets(RepaintBgToSelectionTint(content, s.focused), SelectionTintBgANSI(s.focused)) // fork
	return strings.Split(s.selected.Render(fill.Render(tinted)), "\n")
}

func assertWrapRowsMatchLipgloss(t *testing.T, name string, msg MessageItem, r RenderedMessage, width int, s WrapStyles) {
	t.Helper()
	got, want := WrapRows(msg, r, width, s), wrapRowsLipgloss(msg, r, width, s)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s width=%d focused=%v: WrapRows differs from the lipgloss passes\n got: %q\nwant: %q",
			name, width, s.focused, got.Lines, want.Lines)
		return
	}
	if gotS, wantS := s.SelectedVariant(r.Content, width), selectedVariantLipgloss(s, r.Content, width); !reflect.DeepEqual(gotS, wantS) {
		t.Errorf("%s width=%d focused=%v: SelectedVariant differs from the lipgloss passes\n got: %q\nwant: %q",
			name, width, s.focused, gotS, wantS)
	}
}

// Every message shape the channel and thread panes render wraps to the
// same bytes as the lipgloss passes, at every width and focus.
func TestWrapRows_MatchesLipglossOnRenderedMessages(t *testing.T) {
	msgs := benchRichMessages(t, 20)
	msgs = append(msgs,
		MessageItem{TS: "1700000100.000100", UserName: "eve", UserID: "U5", Timestamp: "10:30 AM", Text: "", IsEdited: true},
		MessageItem{TS: "1700000101.000100", UserName: "eve", UserID: "U5", Timestamp: "10:30 AM", Text: "wide 漢字漢字 and emoji 🎉👍🏽 and a tab\there"},
		MessageItem{TS: "1700000102.000100", UserName: "eve", UserID: "U5", Timestamp: "10:30 AM", Text: strings.Repeat("unbreakable", 30)},
	)
	for _, avatar := range []string{"", "AAAA\nAAAA"} {
		for _, focused := range []bool{true, false} {
			s := NewWrapStyles(focused)
			for _, width := range []int{1, 2, 10, 24, 40, 80, 133, 160} {
				m := New(nil, "general")
				for i, msg := range msgs {
					r := RenderMessage(msg, width, m.renderContext(), RenderOptions{
						Avatar: avatar, ReplyAffordance: true, EditedMark: true, BroadcastLabel: true, ImageHits: true,
					})
					assertWrapRowsMatchLipgloss(t, fmt.Sprintf("msg %d avatar=%q", i, avatar), msg, r, width, s)
				}
			}
		}
	}
}

// Content the direct assembly must not take, because the lipgloss passes
// change it beyond styling and padding, still comes out the same.
func TestWrapRows_MatchesLipglossOnContentItCannotAssemble(t *testing.T) {
	bg := BgANSI()
	cases := map[string]string{
		"plain":                   "hello\nworld",
		"empty":                   "",
		"empty rows":              "\n\n",
		"styled rows":             bg + "hello\x1b[m\n\x1b[1mbold\x1b[0m",
		"tab":                     "a\tb",
		"carriage return":         "a\r\nb",
		"bell":                    "a\ab",
		"too wide":                strings.Repeat("x", 50),
		"too wide words":          strings.Repeat("word ", 20),
		"style open at row end":   "\x1b[1mbold\nstill bold\x1b[m",
		"style open at the end":   "\x1b[1mbold",
		"style closed by 22":      "\x1b[1mbold\x1b[22m\nnext",
		"link closed":             "\x1b]8;;https://e.com\x07link\x1b]8;;\x07\nnext",
		"link closed with ST":     "\x1b]8;;https://e.com\x1b\\link\x1b]8;;\x1b\\\nnext",
		"link open at row end":    "\x1b]8;;https://e.com\x07link\nstill link\x1b]8;;\x07",
		"link with params":        "\x1b]8;id=1;https://e.com\x07link\x1b]8;id=1;\x07",
		"other OSC":               "\x1b]0;title\x07text",
		"other CSI":               "\x1b[2Ktext",
		"lone ESC":                "text\x1b",
		"ESC then letter":         "\x1bctext",
		"ESC then SGR":            "\x1b\x1b[1mbold\nnext\x1b[m",
		"unterminated CSI":        "text\x1b[1",
		"unterminated OSC":        "text\x1b]8;;x",
		"invalid UTF-8":           "a\xffb",
		"C1 NEL byte":             "a\x85b c",
		"C1 CSI byte":             "a\x9b1mb",
		"C1 ST byte":              "a\x9cb",
		"wide":                    "漢字漢字漢字漢字漢字漢字",
		"wide at the edge":        strings.Repeat("x", 37) + "漢",
		"emoji":                   "🎉👍🏽🇺🇸",
		"nbsp":                    "a b",
		"selection tint inside":   SelectionTintBgANSI(true) + "x\x1b[m",
		"trailing spaces":         "text   \n   ",
		"width exactly the limit": strings.Repeat("y", 39),
	}
	for _, focused := range []bool{true, false} {
		s := NewWrapStyles(focused)
		for name, content := range cases {
			for _, width := range []int{1, 2, 10, 40} {
				msg := MessageItem{TS: "1700000000.000100", Timestamp: "10:30 AM"}
				assertWrapRowsMatchLipgloss(t, name, msg, RenderedMessage{Content: content}, width, s)
			}
		}
	}
}
