package messages

import (
	"fmt"
	stdimage "image"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/image/kittytest"
	"github.com/gammons/slk/internal/ui/imgrender"
)

// A channel whose images add up to more than the terminal keeps uploads
// only the images on screen, keeps every image on screen drawn, and
// sends an image again only when it scrolls back into view after the
// terminal dropped it. The thread pane's counterpart is
// TestKitty_ThreadUploadsOnlyTheImagesOnScreen.
func TestKitty_ChannelUploadsOnlyTheImagesOnScreen(t *testing.T) {
	t.Setenv("TMUX", "")
	const n = 64 // 640x640 px each: 100 MiB decoded
	cells, cellPx := stdimage.Pt(80, 40), stdimage.Pt(8, 16)

	var msgs []MessageItem
	var keys []string
	for i := range n {
		id := fmt.Sprintf("F%03d", i)
		msgs = append(msgs, MessageItem{
			TS: fmt.Sprintf("1700000000.%06d", i), UserName: "alice", Text: "shot", Timestamp: "9:00 AM",
			Attachments: []Attachment{{
				Kind: "image", Name: "shot.png", FileID: id, Mime: "image/png",
				Thumbs: []ThumbSpec{{URL: "https://example.com/" + id + ".png", W: 720, H: 720}},
			}},
		})
		keys = append(keys, id+"-720")
	}

	kr := imgpkg.NewKittyRenderer(imgpkg.NewRegistry())
	m := New(msgs, "C1")
	m.SetImageContext(imgrender.ImageContext{
		Protocol:    imgpkg.ProtoKitty,
		Fetcher:     kittytest.Fetcher(t, kr, keys, cells, cellPx),
		KittyRender: kr,
		CellPixels:  cellPx,
		MaxRows:     cells.Y,
	})

	term := kittytest.New(kittytest.HerdrStoreBytes)
	saved := imgpkg.KittyOutput
	imgpkg.KittyOutput = term
	t.Cleanup(func() { imgpkg.KittyOutput = saved })

	seen := map[uint32]bool{}
	frames := 0
	draw := func(step string) {
		t.Helper()
		frames++
		placed, blank, offscreen := term.Frame(m.View(50, 120))
		for id := range placed {
			seen[id] = true
		}
		if len(blank) > 0 {
			t.Errorf("%s: images %v are on screen but the terminal no longer holds them", step, blank)
		}
		// Only the two messages the window's edges cut through can send
		// an image whose own rows are off screen.
		if len(offscreen) > 2 {
			t.Errorf("%s: uploaded %d images that are not on screen (%v), want at most 2", step, len(offscreen), offscreen)
		}
	}

	draw("open") // on the last message
	m.GoToTop()
	draw("g")
	for i := range n {
		m.MoveDown()
		draw(fmt.Sprintf("j #%d", i+1))
	}
	for !m.ViewportAtTop() {
		m.ScrollUp(5)
		draw("wheel up")
	}
	beforeIdle := term.Uploads
	for range 10 {
		draw("idle")
	}

	t.Logf("%d frames: %d uploads, %d MiB decoded, %d MiB written; terminal holds %d MiB",
		frames, term.Uploads, term.UploadedBytes>>20, term.Written>>20, term.Stored()>>20)
	if len(seen) != n {
		t.Fatalf("frames placed %d distinct images, want %d (the fixture did not render)", len(seen), n)
	}
	if limit := 2*n + 2; term.Uploads > limit {
		t.Errorf("%d uploads for %d images over one scroll down and back, want at most %d", term.Uploads, n, limit)
	}
	if idle := term.Uploads - beforeIdle; idle != 0 {
		t.Errorf("redrawing an unchanged frame uploaded %d images, want none", idle)
	}
}
