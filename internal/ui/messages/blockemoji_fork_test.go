package messages

import (
	stdimage "image"
	"io"
	"strings"
	"testing"

	emojiutil "github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// An emoji in a table cell is a kitty placement like one in the body:
// its upload must reach the message's flushes, or the terminal draws
// whatever image it already holds under that id.
func TestRenderMessageTableCellEmojiUploads(t *testing.T) {
	emojiutil.SetImageMode(true, 2)
	t.Cleanup(func() { emojiutil.SetImageMode(false, 2) })

	uploaded := false
	ff := newFakePlaceFetcher()
	ff.setPrerendered(emojiutil.EmojiCacheKey(emojiutil.CDNBaseURL+"1f44d.png"), stdimage.Pt(2, 1), imgpkg.Render{
		Cells:   stdimage.Pt(2, 1),
		Lines:   []string{"\U0010EEEE\U0010EEEE"},
		OnFlush: func(io.Writer) error { uploaded = true; return nil },
	})
	m := New(nil, "general")
	m.SetEmojiContext(EmojiContext{PlaceCtx: emojiutil.PlaceContext{Fetcher: ff}, Cells: 2})

	msg := MessageItem{TS: "1.0", UserName: "claude", Blocks: []blockkit.Block{
		blockkit.TableBlock{Rows: [][]string{{"Status"}, {":thumbsup: done"}}},
	}}
	content, flushes, _, _, _, _, _ := m.renderMessagePlain(msg, 80, "", nil, nil, false, nil)
	if !strings.Contains(content, "\U0010EEEE") {
		t.Fatalf("table cell emoji did not render as a kitty placement:\n%q", content)
	}
	for _, f := range flushes {
		_ = f(io.Discard)
	}
	if !uploaded {
		t.Errorf("table cell emoji placed but its upload is missing from the message's %d flushes", len(flushes))
	}
}
