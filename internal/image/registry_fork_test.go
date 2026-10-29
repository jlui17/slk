package image

import (
	"bytes"
	"fmt"
	"image"
	imgcolor "image/color"
	"strings"
	"testing"
)

// An image the terminal's store has dropped is sent again. herdr drops a
// pane's oldest uploads once later ones pass its store limit, so after
// enough uploads an emoji sent early in a session (a reaction pill's 👍)
// drew blank: its closure had fired once and never sent it again.
func TestKitty_ResendsAnImageOnceLaterUploadsPassTheStoreBudget(t *testing.T) {
	t.Setenv("TMUX", "")
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })
	SetCellPixels(17, 37)

	reg := NewRegistry()
	r := NewKittyRenderer(reg)
	r.SetSource("emoji", makeSolid(64, 64, imgcolor.RGBA{1, 2, 3, 255}))
	emoji := r.RenderKey("emoji", image.Pt(2, 1))
	if emoji.OnFlush == nil {
		t.Fatal("first render should flush (precondition)")
	}
	var first bytes.Buffer
	if err := emoji.OnFlush(&first); err != nil {
		t.Fatal(err)
	}
	var again bytes.Buffer
	if err := emoji.OnFlush(&again); err != nil {
		t.Fatal(err)
	}
	if again.Len() != 0 {
		t.Fatalf("a second flush right after the upload sent %d bytes, want none", again.Len())
	}

	// Each preview is capped at maxKittyUploadBytes of decoded pixels.
	big := makeSolid(1700, 2220, imgcolor.RGBA{9, 9, 9, 255})
	for i := 0; i*maxKittyUploadBytes <= kittyStoreBudgetBytes; i++ {
		key := fmt.Sprintf("preview-%d", i)
		r.SetSource(key, big)
		p := r.RenderKey(key, image.Pt(100, 60))
		if p.OnFlush == nil {
			t.Fatalf("%s: expected a fresh upload", key)
		}
		if err := p.OnFlush(&bytes.Buffer{}); err != nil {
			t.Fatal(err)
		}
	}

	// A pane that renders it again gets an upload too.
	if rerender := r.RenderKey("emoji", image.Pt(2, 1)); rerender.OnFlush == nil {
		t.Error("a render of the dropped emoji should hand back an upload")
	}
	var resent bytes.Buffer
	if err := emoji.OnFlush(&resent); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("a=T,f=100,t=d,i=%d,", emoji.ID); !strings.Contains(resent.String(), want) {
		t.Errorf("the emoji's flush after %d MiB of later uploads sent %d bytes, want an upload of image %d",
			kittyStoreBudgetBytes>>20, resent.Len(), emoji.ID)
	}
}
