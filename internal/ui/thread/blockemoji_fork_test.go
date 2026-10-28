package thread

import (
	stdimage "image"
	"io"
	"slices"
	"strings"
	"testing"

	emojiutil "github.com/gammons/slk/internal/emoji"
	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// Mirrors the messages pane: a table cell emoji's upload reaches the
// reply's flushes.
func TestRenderThreadMessageTableCellEmojiUploads(t *testing.T) {
	emojiutil.SetImageMode(true, 2)
	t.Cleanup(func() { emojiutil.SetImageMode(false, 2) })

	uploaded := false
	ff := newFakePlaceFetcher()
	ff.setPrerendered(emojiutil.EmojiCacheKey(emojiutil.CDNBaseURL+"1f44d.png"), stdimage.Pt(2, 1), imgpkg.Render{
		Cells:   stdimage.Pt(2, 1),
		Lines:   []string{"\U0010EEEE\U0010EEEE"},
		OnFlush: func(io.Writer) error { uploaded = true; return nil },
	})
	m := New()
	m.SetEmojiContext(messages.EmojiContext{PlaceCtx: emojiutil.PlaceContext{Fetcher: ff}, Cells: 2})

	msg := messages.MessageItem{TS: "1.1", UserName: "claude", Blocks: []blockkit.Block{
		blockkit.TableBlock{Rows: [][]string{{"Status"}, {":thumbsup: done"}}},
	}}
	r := m.renderMessage(msg, 80, false)
	got, flushes := r.Content, slices.Concat(r.BodyFlushes, r.BlockFlushes, r.ReactionFlushes, r.AttachmentFlushes)
	if !strings.Contains(got, "\U0010EEEE") {
		t.Fatalf("table cell emoji did not render as a kitty placement:\n%q", got)
	}
	for _, f := range flushes {
		_ = f(io.Discard)
	}
	if !uploaded {
		t.Errorf("table cell emoji placed but its upload is missing from the reply's %d flushes", len(flushes))
	}
}

// The parent message at the top of the panel uploads its table cell
// emoji too: an agent thread's root is where such tables usually are.
func TestThreadParentTableCellEmojiUploads(t *testing.T) {
	emojiutil.SetImageMode(true, 2)
	t.Cleanup(func() { emojiutil.SetImageMode(false, 2) })

	uploaded := false
	ff := newFakePlaceFetcher()
	ff.setPrerendered(emojiutil.EmojiCacheKey(emojiutil.CDNBaseURL+"1f44d.png"), stdimage.Pt(2, 1), imgpkg.Render{
		Cells:   stdimage.Pt(2, 1),
		Lines:   []string{"\U0010EEEE\U0010EEEE"},
		OnFlush: func(io.Writer) error { uploaded = true; return nil },
	})
	m := New()
	m.SetEmojiContext(messages.EmojiContext{PlaceCtx: emojiutil.PlaceContext{Fetcher: ff}, Cells: 2})

	parent := messages.MessageItem{TS: "1.0", UserName: "claude", Blocks: []blockkit.Block{
		blockkit.TableBlock{Rows: [][]string{{"Status"}, {":thumbsup: done"}}},
	}}
	m.SetThread(parent, nil, "C1", "1.0")
	if out := m.View(80, 24); !strings.Contains(out, "\U0010EEEE") {
		t.Fatalf("parent table cell emoji did not render as a kitty placement:\n%q", out)
	}
	if !uploaded {
		t.Error("parent table cell emoji placed but never uploaded")
	}
}
