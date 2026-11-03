package thread

import (
	"fmt"
	stdimage "image"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/image/kittytest"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
)

// A thread whose images add up to more than the terminal keeps uploads
// only the images on screen. It used to send every reply's images on
// each cursor move: past herdr's 64 MiB store the registry counted them
// all as dropped, so every j re-sent the whole thread (9.4 GiB in one
// run on a 105-image thread) and the images sent first drew blank.
func TestKitty_ThreadUploadsOnlyTheImagesOnScreen(t *testing.T) {
	t.Setenv("TMUX", "")
	const replies = 64 // 65 images of 640x640 px: 100 MiB decoded
	cells, cellPx := stdimage.Pt(80, 40), stdimage.Pt(8, 16)

	item := func(ts, fileID string) messages.MessageItem {
		return messages.MessageItem{
			TS: ts, UserName: "alice", Text: "shot", Timestamp: "9:00 AM",
			Attachments: []messages.Attachment{{
				Kind: "image", Name: "shot.png", FileID: fileID, Mime: "image/png",
				Thumbs: []messages.ThumbSpec{{URL: "https://example.com/" + fileID + ".png", W: 720, H: 720}},
			}},
		}
	}
	parent := item("100.000000", "FPARENT")
	keys := []string{"FPARENT-720"}
	var rs []messages.MessageItem
	for i := range replies {
		id := fmt.Sprintf("F%03d", i)
		rs = append(rs, item(fmt.Sprintf("101.%06d", i), id))
		keys = append(keys, id+"-720")
	}

	kr := imgpkg.NewKittyRenderer(imgpkg.NewRegistry())
	m := New()
	m.SetImageContext(imgrender.ImageContext{
		Protocol:    imgpkg.ProtoKitty,
		Fetcher:     kittytest.Fetcher(t, kr, keys, cells, cellPx),
		KittyRender: kr,
		CellPixels:  cellPx,
		MaxRows:     cells.Y,
	})
	m.SetThread(parent, rs, "C1", parent.TS)

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
		// A reply's images go out when any row of it is on screen, so
		// only the two replies the window's edges cut through can send
		// an image whose own rows are off screen.
		if len(offscreen) > 2 {
			t.Errorf("%s: uploaded %d images that are not on screen (%v), want at most 2", step, len(offscreen), offscreen)
		}
	}

	draw("open") // on the last reply
	m.GoToTop()
	draw("g")
	for i := range replies {
		m.MoveDown()
		draw(fmt.Sprintf("j #%d", i+1))
	}
	down := term.Uploads
	for !m.ViewportAtTop() {
		m.ScrollUp(5)
		draw("wheel up")
	}
	beforeIdle := term.Uploads
	for range 10 {
		draw("idle")
	}

	t.Logf("%d frames: %d uploads (%d on the way down), %d MiB decoded, %d MiB written; terminal holds %d MiB",
		frames, term.Uploads, down, term.UploadedBytes>>20, term.Written>>20, term.Stored()>>20)
	if len(seen) != replies+1 {
		t.Fatalf("frames placed %d distinct images, want %d (the fixture did not render)", len(seen), replies+1)
	}
	// Each image goes out once on the way down and at most once more
	// when it scrolls back into view after the terminal dropped it; the
	// last screenful, drawn at open, once more.
	if limit := 2*(replies+1) + 2; term.Uploads > limit {
		t.Errorf("%d uploads for %d images over one scroll down and back, want at most %d",
			term.Uploads, replies+1, limit)
	}
	if idle := term.Uploads - beforeIdle; idle != 0 {
		t.Errorf("redrawing an unchanged frame uploaded %d images, want none", idle)
	}
}
