package image

import (
	"errors"
	"fmt"
	"image"
	"sync"

	"github.com/gammons/slk/internal/debuglog"
)

// fitWithin scales img down until it fits inside box, taking the aspect
// ratio from the decoded bounds. Images already inside box are returned
// as they are.
func fitWithin(img image.Image, box image.Point) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= box.X && h <= box.Y {
		return img
	}
	scale := min(float64(box.X)/float64(w), float64(box.Y)/float64(h))
	return downscale(img, image.Pt(max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))))
}

// ErrUndecodable marks a fetch whose bytes arrived intact but defeated
// every registered decoder. Slack thumbnails HEIC and TIFF uploads to
// JPEG while serving the original untouched, so this describes the file
// rather than a bad moment: a caller can act on it as permanent.
var ErrUndecodable = errors.New("no decoder for image bytes")

// undecodableOriginals remembers which original URLs came back
// undecodable. The disk cache cannot carry that knowledge, because a
// decode failure evicts the entry it would be recorded against, so
// without this every preview open re-downloads the same original in
// full. Process lifetime only: a re-encode on Slack's side, or a decoder
// added in a later build, gets a fresh chance next run.
var undecodableOriginals sync.Map // string(url) -> struct{}

// MarkOriginalUndecodable records that url's bytes defeated every
// registered decoder, so later previews of that file go straight to a
// thumbnail.
func MarkOriginalUndecodable(url string) {
	if url == "" {
		return
	}
	undecodableOriginals.Store(url, struct{}{})
	debuglog.ImgFetch("MarkOriginalUndecodable: url=%s", url)
}

// ResetUndecodableOriginalsForTest clears the recorded decode failures.
// The record is keyed by URL and httptest reuses ports, so without this
// a mark left by one test can land on a later test's server.
func ResetUndecodableOriginalsForTest() {
	undecodableOriginals.Clear()
}

// maxOriginalPixels caps the original the preview is willing to fetch.
// Decoding holds the whole image uncompressed at 4 bytes per pixel, so
// this ceiling is ~160MB of RGBA plus the compressed bytes alongside it,
// allocated on a single keypress. 40 megapixels is where consumer
// hardware stops: it clears every phone, every mirrorless camera, and
// any 6K screenshot, so what it turns away is panoramas and scanned
// documents. For those the thumbnail's softness beats the spike. Raise
// it only if ordinary uploads start landing above the line.
const maxOriginalPixels = 40_000_000

// PickPreviewSource chooses what the full-screen preview should fetch.
//
// The overlay scales its source up to fill the pane, so a thumbnail
// smaller than budget (the pane's size in pixels) renders soft. original
// describes the unresized upload — Slack's url_private plus
// original_w/original_h — and wins when its dimensions are known, beat
// the largest thumb, and that thumb doesn't already cover budget on both
// axes. Otherwise the largest thumb is used, as before.
//
// suffix distinguishes the two in the cache key. The fetcher stores raw
// bytes per key, so original bytes must not come back from a thumb-keyed
// entry once the window grows.
//
// Originals larger than maxOriginalPixels are refused, because decoding
// one allocates the whole image uncompressed before any of it is scaled
// down.
//
// These dimensions only choose a source; they never size the result.
// Slack reports original_w/original_h post-orientation while Go's JPEG
// decoder ignores EXIF orientation, so the two disagree by a 90° turn on
// any auto-oriented camera upload. The fetcher scales from the decoded
// bounds instead (FetchRequest.FitWithin).
func PickPreviewSource(thumbs []ThumbSpec, original ThumbSpec, budget image.Point) (url, suffix string) {
	if _, bad := undecodableOriginals.Load(original.URL); bad {
		debuglog.ImgRender("PickPreviewSource: original=%s known undecodable; thumbs only", original.URL)
		original = ThumbSpec{}
	}

	var largest ThumbSpec
	for _, t := range thumbs {
		if t.URL != "" && max(t.W, t.H) > max(largest.W, largest.H) {
			largest = t
		}
	}

	if original.URL != "" && original.W > 0 && original.H > 0 &&
		original.W*original.H <= maxOriginalPixels &&
		(original.W > largest.W || original.H > largest.H) &&
		(budget.X > largest.W || budget.Y > largest.H) {
		debuglog.ImgRender("PickPreviewSource: chose original=(%d,%d) budget=(%d,%d) largest_thumb=(%d,%d)",
			original.W, original.H, budget.X, budget.Y, largest.W, largest.H)
		return original.URL, "orig"
	}

	if largest.URL == "" {
		debuglog.ImgRender("PickPreviewSource: no source available budget=(%d,%d)", budget.X, budget.Y)
		return "", ""
	}
	debuglog.ImgRender("PickPreviewSource: chose thumb=(%d,%d) budget=(%d,%d) original=(%d,%d)",
		largest.W, largest.H, budget.X, budget.Y, original.W, original.H)
	return largest.URL, fmt.Sprintf("%d", max(largest.W, largest.H))
}
