package image

import (
	"image"
	"strings"
	"testing"
)

// Sixel has no cell-aware placement: the encoded pixel size is the
// on-screen size. An image the layout gave N rows must therefore be
// encoded N*cellHeight pixels tall, or it won't cover those rows and
// the cursor advance won't match the reserved space.

// The encoded raster must scale with the reported cell height: a taller
// cell means more pixels for the same row count.
func TestSixelRender_ScalesWithCellHeight(t *testing.T) {
	resetCellPixels(t)
	t.Cleanup(func() { resetCellPixels(t) })

	src := image.NewRGBA(image.Rect(0, 0, 64, 64))
	target := image.Pt(10, 5)

	SetCellPixels(8, 16)
	small := renderSixelBytes(t, src, target)

	SetCellPixels(16, 32)
	large := renderSixelBytes(t, src, target)

	if len(large) <= len(small) {
		t.Errorf("doubling the cell size did not grow the sixel raster: small=%d large=%d bytes",
			len(small), len(large))
	}
}

func renderSixelBytes(t *testing.T, img image.Image, target image.Point) []byte {
	t.Helper()
	out := (&SixelRenderer{}).Render(img, target)
	if out.OnFlush == nil {
		t.Fatal("Render returned no OnFlush; expected sixel bytes")
	}
	var sb strings.Builder
	if err := out.OnFlush(&sb); err != nil {
		t.Fatalf("OnFlush: %v", err)
	}
	return []byte(sb.String())
}
