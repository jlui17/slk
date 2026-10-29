package image

import (
	"bytes"
	"fmt"
	"image"
	imgcolor "image/color"
	"strings"
	"testing"
)

// renderDroppedEmoji renders and uploads a 2x1 emoji, then uploads more
// than kittyStoreBudgetBytes of previews after it, so the registry counts
// the emoji as dropped by the terminal. It returns the emoji's render and
// the last preview's, which is still held.
func renderDroppedEmoji(t *testing.T, r *KittyRenderer) (emoji, last Render) {
	t.Helper()
	r.SetSource("emoji", makeSolid(64, 64, imgcolor.RGBA{1, 2, 3, 255}))
	emoji = r.RenderKey("emoji", image.Pt(2, 1))
	if emoji.OnFlush == nil {
		t.Fatal("first render should flush (precondition)")
	}
	if err := emoji.OnFlush(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	// Each preview is capped at maxKittyUploadBytes of decoded pixels.
	big := makeSolid(1700, 2220, imgcolor.RGBA{9, 9, 9, 255})
	for i := 0; i*maxKittyUploadBytes <= kittyStoreBudgetBytes; i++ {
		key := fmt.Sprintf("preview-%d", i)
		r.SetSource(key, big)
		last = r.RenderKey(key, image.Pt(100, 60))
		if last.OnFlush == nil {
			t.Fatalf("%s: expected a fresh upload", key)
		}
		if err := last.OnFlush(&bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}
	return emoji, last
}

func setKittyTestCells(t *testing.T) {
	t.Setenv("TMUX", "")
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })
	SetCellPixels(17, 37)
}

// A pane's flushes send their image once. Re-sending dropped images from
// the panes' flushes made the thread pane, which flushes every reply's
// images on each rebuild, send all ~80 images of a 292-reply thread on
// every rebuild once they passed the store budget: 12 GiB in 24 s, which
// pushed the on-screen images out of herdr's store and left them blank.
func TestKitty_AFlushSendsItsImageOnlyOnce(t *testing.T) {
	setKittyTestCells(t)
	r := NewKittyRenderer(NewRegistry())
	emoji, _ := renderDroppedEmoji(t, r)

	var again bytes.Buffer
	if err := emoji.OnFlush(&again); err != nil {
		t.Fatal(err)
	}
	if again.Len() != 0 {
		t.Errorf("a second flush of the emoji sent %d bytes, want none", again.Len())
	}
	if rerender := r.RenderKey("emoji", image.Pt(2, 1)); rerender.OnFlush != nil {
		t.Error("a render of an uploaded emoji handed back an upload, want none")
	}
}

// An image the terminal has dropped is sent again when a frame shows it.
// herdr keeps 64 MiB of a pane's images and drops the oldest past that,
// even ones on screen, so an emoji sent early drew blank.
func TestKitty_ResendDroppedSendsTheDroppedImagesTheFrameShows(t *testing.T) {
	setKittyTestCells(t)
	r := NewKittyRenderer(NewRegistry())
	emoji, last := renderDroppedEmoji(t, r)

	// A reaction pill: the pill style's background sits between the
	// placeholder's foreground and its cells.
	pill := strings.Replace(emoji.Lines[0], "m"+string(PlaceholderRune), "m\x1b[48;2;22;27;34m"+string(PlaceholderRune), 1)
	frame := "\x1b[1mhello\x1b[0m " + pill + "\x1b[38;2;139;148;158m1\x1b[0m\n" + strings.Join(last.Lines, "\n")

	var out bytes.Buffer
	if err := r.ResendDropped(&out, frame); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("a=T,f=100,t=d,i=%d,", emoji.ID); !strings.Contains(out.String(), want) {
		t.Errorf("the frame's dropped emoji was not sent again; got %d bytes, want an upload of image %d", out.Len(), emoji.ID)
	}
	if n := strings.Count(out.String(), "a=T,"); n != 1 {
		t.Errorf("sent %d uploads, want 1: only the dropped emoji (the preview on screen is still held, earlier previews are off screen)", n)
	}

	var again bytes.Buffer
	if err := r.ResendDropped(&again, frame); err != nil {
		t.Fatal(err)
	}
	if again.Len() != 0 {
		t.Errorf("the next frame sent %d bytes, want none: the emoji was just sent", again.Len())
	}
}
