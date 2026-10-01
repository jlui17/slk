package messages

import (
	"context"
	"flag"
	"fmt"
	"image"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// Re-bless with: tools/go.sh test ./internal/ui/messages -run TestAttachmentCardRenders -update
var updateCardRenders = flag.Bool("update", false, "rewrite testdata/attachment_cards.txt from current output")

const cardRendersPath = "testdata/attachment_cards.txt"

type cardRenderCase struct {
	caption  string
	msg      MessageItem
	expanded bool
	selected bool
	image    bool
}

type cardImageFetcher struct{ img image.Image }

func (f cardImageFetcher) Fetch(context.Context, imgpkg.FetchRequest) (imgpkg.FetchResult, error) {
	return imgpkg.FetchResult{}, nil
}
func (f cardImageFetcher) Cached(string, image.Point) (image.Image, bool) { return f.img, true }
func (f cardImageFetcher) Prerendered(string, image.Point, imgpkg.Protocol) (imgpkg.Render, bool) {
	return imgpkg.Render{}, false
}
func (f cardImageFetcher) ConfigurePrerender(imgpkg.Protocol)            {}
func (f cardImageFetcher) ConfigurePrerenderKitty(*imgpkg.KittyRenderer) {}

func cardRenderCases() []cardRenderCase {
	quoted := cardMessage("1736093400.000100", linkedMessageCard("?thread_ts=1736000000.000100&cid=C0000000001", cardParagraphs(12)))
	quoted.Text = "&gt; the rollout held at every step\n!note - (<" + cardPermalink("?thread_ts=1736000000.000100&amp;cid=C0000000001") + ">) not sure the last step waited long enough, see <https://example.slack.com/archives/C0000000002/p1736000000000300> for the other one"

	short := cardMessage("1736093400.000200", linkedMessageCard("", "All ten steps are out."))

	two := cardMessage("1736093400.000300",
		linkedMessageCard("?thread_ts=1736000000.000100&cid=C0000000001", cardParagraphs(12)),
		linkedMessageCard("", cardParagraphs(5)))

	bot := MessageItem{TS: "1736093400.000400", UserID: "U3", UserName: "deploybot", Timestamp: "4:11 PM", Text: "deploy finished",
		LegacyAttachments: []blockkit.LegacyAttachment{{
			Color:   "danger",
			Pretext: "Heads up: the canary pool is degraded",
			Title:   "Deploy 421 rolled back",
			Text:    cardParagraphs(3),
			Fields: []blockkit.LegacyField{
				{Title: "Service", Value: "checkout", Short: true},
				{Title: "Region", Value: "us-east-1", Short: true},
				{Title: "Notes", Value: "The rollback finished cleanly and no request was dropped."},
			},
			Footer: "deploybot",
			TS:     time.Date(2025, 1, 5, 16, 2, 0, 0, time.UTC).Unix(), // upstream draws a footer time in UTC
		}}}

	pictured := MessageItem{TS: "1736093400.000500", UserID: "U3", UserName: "deploybot", Timestamp: "4:12 PM", Text: "dashboard snapshot",
		LegacyAttachments: []blockkit.LegacyAttachment{{
			Title:    "Latency during the rollout",
			Text:     "p99 stayed under the objective.",
			ImageURL: "https://example.com/latency.png",
			Footer:   "dashboards",
		}}}

	longPictured := pictured
	longPictured.TS = "1736093400.000600"
	longPictured.LegacyAttachments = []blockkit.LegacyAttachment{{
		Title:    "Latency during the rollout",
		Text:     cardParagraphs(4),
		ImageURL: "https://example.com/latency.png",
		Footer:   "dashboards",
	}}

	return []cardRenderCase{
		{caption: "(a) a > quote, a permalink in the text, and a long linked-message card, collapsed", msg: quoted},
		{caption: "(b) the same, expanded", msg: quoted, expanded: true},
		{caption: "(c) a short linked-message card: no fold", msg: short},
		{caption: "(d) two long cards, collapsed", msg: two},
		{caption: "(e1) a long bot card with color, pretext, title, fields and footer, collapsed", msg: bot},
		{caption: "(e2) the same, expanded", msg: bot, expanded: true},
		{caption: "(f) short text and an image: shown in full, an image's rows do not count toward the fold (half-block image)", msg: pictured, image: true},
		{caption: "(f3) long text and an image, collapsed: the fold hides the image with the rest", msg: longPictured, image: true},
		{caption: "(f3) the same, expanded", msg: longPictured, image: true, expanded: true},
		{caption: "(g) case (a) as the selected message", msg: quoted, selected: true},
	}
}

func renderCardCase(c cardRenderCase, width int) []string {
	m := newCardModel(c.msg)
	if c.image {
		m.SetImageContext(imgrender.ImageContext{
			Protocol:   imgpkg.ProtoHalfBlock,
			Fetcher:    cardImageFetcher{img: image.NewRGBA(image.Rect(0, 0, 160, 90))},
			CellPixels: image.Pt(8, 16),
			MaxRows:    8,
		})
	}
	m.View(200, width)
	if c.expanded {
		m.ToggleAttachmentFold()
		m.View(200, width)
	}
	entry := m.cache[len(m.cache)-1]
	lines := entry.linesNormal
	if c.selected {
		lines = entry.linesSelected
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.TrimRight(ansi.Strip(l), " ")
	}
	return out
}

// The cards as the messages pane draws them, ANSI stripped, at two
// widths: a picture to review a change against.
func TestAttachmentCardRenders(t *testing.T) {
	SetNowFunc(func() time.Time { return time.Date(2025, 1, 6, 9, 0, 0, 0, time.Local) })
	defer SetNowFunc(nil)

	var b strings.Builder
	for _, width := range []int{100, 60} {
		for _, c := range cardRenderCases() {
			fmt.Fprintf(&b, "=== width %d: %s\n", width, c.caption)
			for _, l := range renderCardCase(c, width) {
				b.WriteString(l + "\n")
			}
			b.WriteString("\n")
		}
	}
	got := b.String()

	if *updateCardRenders {
		if err := os.WriteFile(cardRendersPath, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(cardRendersPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("%s is stale; re-bless with -update and review the diff.\ngot:\n%s", cardRendersPath, got)
	}
}
