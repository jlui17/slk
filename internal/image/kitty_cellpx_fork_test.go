package image

import (
	"bytes"
	"encoding/base64"
	"image"
	imgcolor "image/color"
	"image/png"
	"strings"
	"testing"
)

// Kitty stretches the transmitted raster over the c=<cols>,r=<rows>
// placement box, so the payload must be encoded at the terminal's real
// cell pixel size — anything smaller is resolution the terminal cannot
// put back.

func TestKitty_PayloadUsesMeasuredCellPixels(t *testing.T) {
	t.Setenv("TMUX", "")
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })
	SetCellPixels(17, 37)

	target := image.Pt(10, 5)
	r := NewKittyRenderer(NewRegistry())
	r.SetSource("measured", makeSolid(600, 600, imgcolor.RGBA{1, 2, 3, 255}))

	w, h := kittyPayloadDims(t, r.RenderKey("measured", target))
	if w != target.X*17 || h != target.Y*37 {
		t.Errorf("payload = %dx%d px, want %dx%d (cells × measured cell size)",
			w, h, target.X*17, target.Y*37)
	}
}

// The payload memo is keyed on the cell metrics as well as (id, cells):
// a second render after the terminal reports a different cell size must
// re-encode rather than serve the raster built for the old size.
func TestKitty_PayloadMemoDiscriminatesCellMetrics(t *testing.T) {
	t.Setenv("TMUX", "")
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })

	target := image.Pt(4, 2)
	r := NewKittyRenderer(NewRegistry())
	r.SetSource("remeasured", makeSolid(600, 600, imgcolor.RGBA{1, 2, 3, 255}))

	// Neither render is flushed before the next one is taken, so the
	// registry keeps reporting fresh=true and both carry a payload.
	SetCellPixels(8, 16)
	before := r.RenderKey("remeasured", target)
	SetCellPixels(16, 32)
	after := r.RenderKey("remeasured", target)

	if w, h := kittyPayloadDims(t, before); w != target.X*8 || h != target.Y*16 {
		t.Errorf("first payload = %dx%d px, want %dx%d", w, h, target.X*8, target.Y*16)
	}
	if w, h := kittyPayloadDims(t, after); w != target.X*16 || h != target.Y*32 {
		t.Errorf("payload after the cell size changed = %dx%d px, want %dx%d (stale memo?)",
			w, h, target.X*16, target.Y*32)
	}
}

// A full-pane preview of a wide image is the largest raster slk uploads:
// a 1400x888 image in a 244x65 terminal fits a 195x62 cell box, which is
// 3315x2294 px (29 MiB decoded) at a 17x37 cell. The upload must stay
// within maxKittyUploadBytes, and keep the cell box's shape so the
// terminal's stretch over c=<cols>,r=<rows> does not distort it.
func TestKitty_PayloadBoundedForFullPanePreview(t *testing.T) {
	t.Setenv("TMUX", "")
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })
	SetCellPixels(17, 37)

	target := fitInto(1400, 888, 205, 62)
	r := NewKittyRenderer(NewRegistry())
	r.SetSource("wide", makeSolid(1400, 888, imgcolor.RGBA{1, 2, 3, 255}))

	w, h := kittyPayloadDims(t, r.RenderKey("wide", target))
	if decoded := w * h * 4; decoded > maxKittyUploadBytes {
		t.Errorf("payload = %dx%d px, %d bytes decoded, want at most %d",
			w, h, decoded, maxKittyUploadBytes)
	}
	boxW, boxH := target.X*17, target.Y*37
	if got, want := float64(w)/float64(h), float64(boxW)/float64(boxH); got < want*0.99 || got > want*1.01 {
		t.Errorf("payload %dx%d has aspect %.4f, want the cell box's %.4f", w, h, got, want)
	}
}

// kittyPayloadDims flushes r and reports the pixel dimensions of the PNG
// carried by the emitted kitty upload.
func kittyPayloadDims(t *testing.T, r Render) (int, int) {
	t.Helper()
	if r.OnFlush == nil {
		t.Fatal("expected OnFlush carrying an upload payload")
	}
	var buf bytes.Buffer
	if err := r.OnFlush(&buf); err != nil {
		t.Fatalf("OnFlush: %v", err)
	}

	var b64 strings.Builder
	for _, seq := range strings.Split(buf.String(), "\x1b_G") {
		body, ok := strings.CutSuffix(seq, "\x1b\\")
		if !ok {
			continue // leading empty split element
		}
		_, chunk, ok := strings.Cut(body, ";")
		if !ok {
			t.Fatalf("kitty sequence has no payload separator: %q", body)
		}
		b64.WriteString(chunk)
	}

	dec := base64.NewDecoder(base64.StdEncoding, strings.NewReader(b64.String()))
	cfg, err := png.DecodeConfig(dec)
	if err != nil {
		t.Fatalf("decode payload PNG: %v", err)
	}
	return cfg.Width, cfg.Height
}
