package image

import (
	"bytes"
	"compress/zlib"
	"image"
	imgpng "image/png"
	"sync/atomic"

	"github.com/gammons/slk/internal/debuglog"
)

// kittyUploadRGBA switches every kitty transmit from PNG (f=100) to
// zlib-compressed raw RGBA (f=32,o=z). Set once at startup, before any
// rendering goroutine runs (same publication contract as cellPxW/H),
// when the terminal's graphics implementation rejects PNG but acks raw
// pixels — libghostty-vt embedders without a wired PNG decoder, e.g.
// herdr (probed via ProbeKittyGraphics / ProbeKittyRGBA).
var kittyUploadRGBA atomic.Bool

// SetKittyUploadRGBA switches kitty uploads to raw RGBA encoding.
func SetKittyUploadRGBA() {
	kittyUploadRGBA.Store(true)
	debuglog.ImgRender("kitty uploads: raw RGBA (terminal rejects PNG)")
}

// payloadKey scopes the payload memo. The raster is resampled to
// cells × cell-pixel-size, so the same (id, target) encodes a
// different payload once the terminal reports a different cell size.
type payloadKey struct {
	placeholderKey
	cellPxW int
	cellPxH int
}

// kittyPayload is a memoized upload: the base64 bytes plus the pixel
// format they were encoded in, so the emit header can never disagree
// with the encoding.
type kittyPayload struct {
	b64  string
	rgba bool
}

// encodeKittyPayload encodes the resized raster: PNG by default,
// zlib-compressed raw RGBA when rgba is set. The raw path serializes
// img.Pix directly — img is always freshly allocated by image.NewRGBA
// at (0,0), so Pix is contiguous with no stride padding.
func encodeKittyPayload(img *image.RGBA, rgba bool) ([]byte, error) {
	var buf bytes.Buffer
	if rgba {
		zw := zlib.NewWriter(&buf)
		if _, err := zw.Write(img.Pix); err != nil {
			return nil, err
		}
		if err := zw.Close(); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	if err := imgpng.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
