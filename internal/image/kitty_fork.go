package image

import (
	"image"
	"math"
)

// payloadKey scopes the payload memo. The raster is resampled to
// cells × cell-pixel-size, so the same (id, target) encodes a
// different PNG once the terminal reports a different cell size.
type payloadKey struct {
	placeholderKey
	cellPxW int
	cellPxH int
}

// maxKittyUploadBytes bounds one upload's decoded RGBA size. herdr
// (0.9.x) decodes a pane's image and forwards the pixels to the outer
// terminal, and silently draws nothing for an image over its transfer
// limits: 16 MiB for the temp-file path to a local terminal (MAX_FILE in
// its src/server/headless/native_graphics.rs), about 22 MiB inline
// (HEADLESS_GRAPHICS_TRANSACTION_BUDGET in src/kitty_graphics.rs). A
// full-pane preview at a HiDPI cell size is well past both.
const maxKittyUploadBytes = 16 << 20

// kittyUploadPixels is the raster size for a target cell box: cells ×
// cell pixels, scaled down in shape when that exceeds
// maxKittyUploadBytes. The terminal stretches the raster over the cell
// box either way, so the cap costs sharpness, never layout.
func kittyUploadPixels(target image.Point, cellPxW, cellPxH int) (pxW, pxH int) {
	pxW, pxH = target.X*cellPxW, target.Y*cellPxH
	if decoded := pxW * pxH * 4; decoded > maxKittyUploadBytes {
		scale := math.Sqrt(float64(maxKittyUploadBytes) / float64(decoded))
		pxW, pxH = int(float64(pxW)*scale), int(float64(pxH)*scale)
	}
	return pxW, pxH
}

// kittyUploadBytes is the decoded size of the raster uploaded for a
// target cell box, what a terminal's image store counts.
func kittyUploadBytes(target image.Point, cellPxW, cellPxH int) int {
	pxW, pxH := kittyUploadPixels(target, cellPxW, cellPxH)
	return pxW * pxH * 4
}

// RenderSource is SetSource then RenderKey, except the renderer lets go
// of img once the render is built. RenderKey reads a source only to
// encode a payload, and every caller sets the source just before, so
// keeping it in sources only pins the decoded raster for the session:
// a few MB per image at a HiDPI cell size. key must not be shared with
// another goroutine's SetSource, or that goroutine's RenderKey can find
// the source already gone.
func (k *KittyRenderer) RenderSource(key string, img image.Image, target image.Point) Render {
	k.SetSource(key, img)
	r := k.RenderKey(key, target)
	k.mu.Lock()
	delete(k.sources, key)
	k.mu.Unlock()
	return r
}
