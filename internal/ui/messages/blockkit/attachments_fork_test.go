package blockkit

import (
	"context"
	"fmt"
	"image"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
)

func inCardFrame(plain string) bool {
	return strings.HasPrefix(plain, "╭") || strings.HasPrefix(plain, "│") || strings.HasPrefix(plain, "╰")
}

func expandedCtx() Context {
	ctx := makeCtx()
	ctx.Card.Expanded = true
	return ctx
}

// cells strips the frame: what each row of a card says.
func cells(r RenderResult) []string {
	lines := plainLines(r)
	for i, l := range lines {
		lines[i] = strings.TrimSpace(strings.Trim(l, "│╭╮╰╯─"))
	}
	return lines
}

func bodyOf(n int) string {
	lines := make([]string, n)
	for i := range lines {
		lines[i] = fmt.Sprintf("body line %d", i+1)
	}
	return strings.Join(lines, "\n")
}

// linkedMessage is the shape of a Slack message unfurl: the author, the
// permalink in from_url, the body flat in text and formatted in blocks.
func linkedMessage(at time.Time, query, body string) LegacyAttachment {
	return LegacyAttachment{
		AuthorName: "Claude",
		FromURL:    fmt.Sprintf("https://example.slack.com/archives/C0000000001/p%d000000%s", at.Unix(), query),
		Footer:     "Slack Conversation",
		TS:         at.Unix(),
		Text:       body,
		Blocks:     []Block{richTextBlockOf(body)},
	}
}

var linkedAt = time.Date(2025, 1, 5, 16, 2, 0, 0, time.Local)

const linkedAtLabel = "Jan 5 2025, 4:02 PM"

func linkedCtx() Context {
	ctx := makeCtx()
	ctx.Card.ChannelNames = map[string]string{"C0000000001": "dev"}
	ctx.Card.Now = time.Date(2026, 3, 1, 9, 0, 0, 0, time.Local)
	return ctx
}

// A legacy attachment has no host-rendered body, so a rich_text block
// nested in it is drawn by Render itself.
func TestRenderLegacyDrawsNestedRichText(t *testing.T) {
	r := RenderLegacy([]LegacyAttachment{{Blocks: []Block{richTextBlockOf("unfurl body")}}}, makeCtx(), 80)
	if !strings.Contains(ansi.Strip(strings.Join(r.Lines, "\n")), "unfurl body") {
		t.Errorf("nested rich_text missing from legacy attachment render: %q", r.Lines)
	}
}

// Slack sends a message unfurl's body twice: flat in text, formatted in
// blocks.
func TestRenderLegacyDrawsLinkedMessageOnce(t *testing.T) {
	r := RenderLegacy([]LegacyAttachment{{
		Text:   "unfurl body",
		Blocks: []Block{richTextBlockOf("unfurl body")},
	}}, makeCtx(), 80)
	plain := ansi.Strip(strings.Join(r.Lines, "\n"))
	if n := strings.Count(plain, "unfurl body"); n != 1 {
		t.Errorf("body drawn %d times, want once:\n%s", n, plain)
	}
}

func TestRenderLegacyKeepsTextWhenBlocksCarryNoBody(t *testing.T) {
	r := RenderLegacy([]LegacyAttachment{{Text: "only copy", Blocks: []Block{DividerBlock{}}}}, makeCtx(), 80)
	if !strings.Contains(ansi.Strip(strings.Join(r.Lines, "\n")), "only copy") {
		t.Errorf("text dropped though no block carries the body: %q", r.Lines)
	}
}

func TestCardIsAFullWidthRoundBox(t *testing.T) {
	for _, width := range []int{100, 60} {
		ctx := expandedCtx()
		ctx.WrapText = wrapWords
		r := RenderLegacy([]LegacyAttachment{{Title: "T", Text: strings.Repeat("word ", 40)}}, ctx, width)
		lines := plainLines(r)
		if len(lines) < 4 {
			t.Fatalf("width %d: body did not wrap inside the box: %q", width, lines)
		}
		for i, l := range lines {
			if w := lipgloss.Width(l); w != width {
				t.Errorf("width %d: row %d is %d wide: %q", width, i, w, l)
			}
			open, shut := "│ ", " │"
			switch i {
			case 0:
				open, shut = "╭─", "─╮"
			case len(lines) - 1:
				open, shut = "╰─", "─╯"
			}
			if !strings.HasPrefix(l, open) || !strings.HasSuffix(l, shut) {
				t.Errorf("width %d: row %d = %q, want %q … %q", width, i, l, open, shut)
			}
			if strings.Contains(l, "█") {
				t.Errorf("width %d: row %d still wears the stripe: %q", width, i, l)
			}
		}
		if r.Height != len(r.Lines) {
			t.Errorf("Height = %d, want %d", r.Height, len(r.Lines))
		}
	}
}

func TestCardBorderTakesTheCardColorElseMuted(t *testing.T) {
	top := func(color string) string {
		return RenderLegacy([]LegacyAttachment{{Color: color, Title: "T"}}, makeCtx(), 20).Lines[0]
	}
	rule := "╭" + strings.Repeat("─", 18) + "╮"
	if got, want := top("#ff8800"), lipgloss.NewStyle().Foreground(lipgloss.Color("#ff8800")).Render(rule); got != want {
		t.Errorf("colored card border = %q, want %q", got, want)
	}
	for _, color := range []string{"", "not-a-color"} {
		if got, want := top(color), mutedStyle().Render(rule); got != want {
			t.Errorf("color %q: border = %q, want muted %q", color, got, want)
		}
	}
}

func TestLinkedMessageHeader(t *testing.T) {
	root := linkedMessage(linkedAt, "", "hi")
	root.Footer = "Thread in Slack Conversation"
	cases := []struct {
		name string
		att  LegacyAttachment
		want string
	}{
		{"reply", linkedMessage(linkedAt, "?thread_ts=1736000000.000100&cid=C0000000001", "hi"), "Claude · Thread in #dev · " + linkedAtLabel},
		{"channel message", linkedMessage(linkedAt, "", "hi"), "Claude · #dev · " + linkedAtLabel},
		{"thread root", root, "Claude · Thread in #dev · " + linkedAtLabel},
	}
	for _, c := range cases {
		got := cells(RenderLegacy([]LegacyAttachment{c.att}, linkedCtx(), 80))
		if want := []string{"", c.want, "hi", ""}; !slices.Equal(got, want) {
			t.Errorf("%s: card = %q, want %q", c.name, got, want)
		}
	}
}

func TestLinkedMessageHeaderWithoutAChannelName(t *testing.T) {
	ctx := linkedCtx()
	ctx.Card.ChannelNames = nil
	got := cells(RenderLegacy([]LegacyAttachment{linkedMessage(linkedAt, "?thread_ts=1736000000.000100", "hi")}, ctx, 80))
	if want := "Claude · Thread · " + linkedAtLabel; got[1] != want {
		t.Errorf("header = %q, want %q", got[1], want)
	}
}

func TestLinkedMessageHeaderAuthorIsBold(t *testing.T) {
	r := RenderLegacy([]LegacyAttachment{linkedMessage(linkedAt, "", "hi")}, linkedCtx(), 80)
	if want := lipgloss.NewStyle().Bold(true).Render("Claude"); !strings.Contains(r.Lines[1], want) {
		t.Errorf("header %q lacks bold author %q", r.Lines[1], want)
	}
}

func TestBotCardKeepsItsFooter(t *testing.T) {
	got := cells(RenderLegacy([]LegacyAttachment{{Title: "T", Footer: "Datadog", FromURL: "https://example.com/x"}}, makeCtx(), 60))
	if want := []string{"", "T", "Datadog", ""}; !slices.Equal(got, want) {
		t.Errorf("card = %q, want %q", got, want)
	}
}

func TestLinkedMessageTimeIsLocalAndDatedLikeSearchResults(t *testing.T) {
	at := time.Date(2026, 3, 1, 16, 2, 0, 0, time.Local)
	ts := fmt.Sprintf("%d.000100", at.Unix())
	cases := []struct {
		now          time.Time
		format, want string
	}{
		{time.Date(2026, 3, 1, 23, 0, 0, 0, time.Local), "", "4:02 PM"},
		{time.Date(2026, 3, 2, 0, 5, 0, 0, time.Local), "", "Mar 1, 4:02 PM"},
		{time.Date(2027, 1, 1, 0, 0, 0, 0, time.Local), "15:04", "Mar 1 2026, 16:02"},
	}
	for _, c := range cases {
		if got := linkedMessageTime(ts, c.now, c.format); got != c.want {
			t.Errorf("now %v: got %q, want %q", c.now, got, c.want)
		}
	}
}

func TestCardFold(t *testing.T) {
	card := func(bodyLines int) []LegacyAttachment {
		return []LegacyAttachment{{Pretext: "Heads up", Title: "Deploy", Text: bodyOf(bodyLines)}}
	}

	r := RenderLegacy(card(5), makeCtx(), 60)
	want := []string{"Heads up", "", "Deploy", "body line 1", "body line 2", "body line 3", "body line 4", "body line 5", ""}
	if got := cells(r); !slices.Equal(got, want) || len(r.FoldRows) != 0 {
		t.Errorf("5 body lines: card = %q folds %v, want %q and no fold row", got, r.FoldRows, want)
	}

	r = RenderLegacy(card(6), makeCtx(), 60)
	want = []string{"Heads up", "", "Deploy", "body line 1", "body line 2", "body line 3 …", "▸ 3 more lines · z to expand", ""}
	if got := cells(r); !slices.Equal(got, want) || !slices.Equal(r.FoldRows, []int{6}) {
		t.Errorf("collapsed: card = %q folds %v, want %q folds [6]", got, r.FoldRows, want)
	}
	if muted := mutedStyle().Render("▸ 3 more lines · z to expand"); !strings.Contains(r.Lines[6], muted) {
		t.Errorf("fold row %q is not muted", r.Lines[6])
	}

	r = RenderLegacy(card(6), expandedCtx(), 60)
	want = []string{"Heads up", "", "Deploy", "body line 1", "body line 2", "body line 3", "body line 4", "body line 5", "body line 6", "▾ z to collapse", ""}
	if got := cells(r); !slices.Equal(got, want) || !slices.Equal(r.FoldRows, []int{9}) {
		t.Errorf("expanded: card = %q folds %v, want %q folds [9]", got, r.FoldRows, want)
	}
}

func TestEveryCardOfAMessageReportsItsFoldRow(t *testing.T) {
	long := LegacyAttachment{Text: bodyOf(8)}
	r := RenderLegacy([]LegacyAttachment{long, {Text: "short"}, long}, makeCtx(), 60)
	if len(r.FoldRows) != 2 {
		t.Fatalf("FoldRows = %v, want two", r.FoldRows)
	}
	for _, row := range r.FoldRows {
		if got := cells(r)[row]; got != "▸ 5 more lines · z to expand" {
			t.Errorf("row %d is not a fold row: %q", row, got)
		}
	}
}

type cachedFetcher struct{ img image.Image }

func (f cachedFetcher) Fetch(context.Context, imgpkg.FetchRequest) (imgpkg.FetchResult, error) {
	return imgpkg.FetchResult{}, nil
}
func (f cachedFetcher) Cached(string, image.Point) (image.Image, bool) { return f.img, true }
func (f cachedFetcher) Prerendered(string, image.Point, imgpkg.Protocol) (imgpkg.Render, bool) {
	return imgpkg.Render{}, false
}
func (f cachedFetcher) ConfigurePrerender(imgpkg.Protocol)            {}
func (f cachedFetcher) ConfigurePrerenderKitty(*imgpkg.KittyRenderer) {}

func imageCtx(proto imgpkg.Protocol) Context {
	ctx := makeCtx()
	ctx.Protocol = proto
	ctx.Fetcher = cachedFetcher{img: image.NewRGBA(image.Rect(0, 0, 160, 90))}
	ctx.CellPixels = image.Pt(8, 16)
	ctx.MaxRows = 8
	if proto == imgpkg.ProtoKitty {
		ctx.KittyRender = imgpkg.NewKittyRenderer(imgpkg.NewRegistry())
	}
	return ctx
}

const cardImageURL = "https://example.com/i.png"

// An image's rows do not count toward the fold: a card with little text
// shows its image, collapsed or not.
func TestCardImageRendersInsideTheFrame(t *testing.T) {
	for _, proto := range []imgpkg.Protocol{imgpkg.ProtoHalfBlock, imgpkg.ProtoKitty} {
		const width = 30
		r := RenderLegacy([]LegacyAttachment{{Title: "T", Text: "caption", ImageURL: cardImageURL}}, imageCtx(proto), width)
		if len(r.Hits) != 1 || len(r.FoldRows) != 0 {
			t.Fatalf("%v: hits = %v folds = %v, want one image and no fold row", proto, r.Hits, r.FoldRows)
		}
		h := r.Hits[0]
		if h.ColStart != cardFrameCols || h.ColEnd > width-cardFrameCols {
			t.Errorf("%v: image cols %d..%d leave the frame's inside %d..%d", proto, h.ColStart, h.ColEnd, cardFrameCols, width-cardFrameCols)
		}
		// top border, title, caption, then the image, then the bottom border
		if h.RowStart != 3 || h.RowEnd-h.RowStart < 6 || h.RowEnd != len(r.Lines)-1 {
			t.Errorf("%v: image rows %d..%d of %d, want rows 3 up to the bottom border", proto, h.RowStart, h.RowEnd, len(r.Lines))
		}
		for row := h.RowStart; row < h.RowEnd; row++ {
			plain := ansi.Strip(r.Lines[row])
			if !strings.HasPrefix(plain, "│ ") || !strings.HasSuffix(plain, " │") || lipgloss.Width(r.Lines[row]) != width {
				t.Errorf("%v: image row %d breaks the frame (%d wide): %q", proto, row, lipgloss.Width(r.Lines[row]), plain)
			}
		}
		if proto == imgpkg.ProtoKitty && len(r.Flushes) == 0 {
			t.Errorf("kitty image lost its upload flush")
		}
	}
}

// Long text folds, and the fold hides the image with the rest.
func TestFoldedCardHidesItsImage(t *testing.T) {
	card := []LegacyAttachment{{Title: "T", Text: bodyOf(6), ImageURL: cardImageURL}}
	for _, proto := range []imgpkg.Protocol{imgpkg.ProtoHalfBlock, imgpkg.ProtoKitty} {
		expanded := imageCtx(proto)
		expanded.Card.Expanded = true
		full := RenderLegacy(card, expanded, 40)
		if len(full.Hits) != 1 || cells(full)[full.Hits[0].RowEnd] != "▾ z to collapse" {
			t.Fatalf("%v: expanded card must show the image above the collapse row: %q", proto, cells(full))
		}
		imageRows := full.Hits[0].RowEnd - full.Hits[0].RowStart

		r := RenderLegacy(card, imageCtx(proto), 40)
		want := []string{"", "T", "body line 1", "body line 2", "body line 3 …", fmt.Sprintf("▸ %d more lines · z to expand", 3+imageRows), ""}
		if got := cells(r); !slices.Equal(got, want) {
			t.Errorf("%v: collapsed card = %q, want %q", proto, got, want)
		}
		if len(r.Hits) != 0 || len(r.Flushes) != 0 {
			t.Errorf("%v: a hidden image left hits %v or %d flushes", proto, r.Hits, len(r.Flushes))
		}
	}
}

// The preview never cuts an image: it ends before one that sits in its
// first three rows.
func TestFoldPreviewEndsBeforeAnImage(t *testing.T) {
	card := []LegacyAttachment{{Blocks: []Block{
		SectionBlock{Text: "intro"},
		ImageBlock{URL: cardImageURL},
		SectionBlock{Text: bodyOf(6)},
	}}}
	r := RenderLegacy(card, imageCtx(imgpkg.ProtoHalfBlock), 40)
	got := cells(r)
	if len(got) != 4 || got[1] != "intro …" || !strings.HasPrefix(got[2], "▸ ") || len(r.Hits) != 0 {
		t.Errorf("collapsed card = %q hits %v, want the intro, then the fold row", got, r.Hits)
	}
}
