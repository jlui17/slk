package image

import (
	"image"
	"io"
	"math"
	"slices"
	"strings"

	"github.com/gammons/slk/internal/debuglog"
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

// kittyUpload is what sending an image again takes: its encoded payload,
// its cell box, and the decoded bytes a terminal's store counts for it.
type kittyUpload struct {
	payload     string
	cols, rows  int
	decodedSize int
}

// rememberUpload records id's upload so ResendDropped can send it again.
func (k *KittyRenderer) rememberUpload(id uint32, payload string, target image.Point, cellPxW, cellPxH int) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.uploads == nil {
		k.uploads = map[uint32]kittyUpload{}
	}
	k.uploads[id] = kittyUpload{payload, target.X, target.Y, kittyUploadBytes(target, cellPxW, cellPxH)}
}

// ResendDropped sends again each image whose placeholder cells frame
// shows and the terminal may have dropped (Registry.evicted). It is the
// only path that sends an image twice: a pane's flush sends its image
// once, because the thread pane flushes every reply's images on each
// rebuild, and re-sending from there sent a long thread's images over
// and over.
func (k *KittyRenderer) ResendDropped(w io.Writer, frame string) error {
	for _, id := range placedImageIDs(frame) {
		if !k.registry.evicted(id) {
			continue
		}
		k.mu.Lock()
		up, ok := k.uploads[id]
		k.mu.Unlock()
		if !ok {
			continue
		}
		debuglog.ImgRender("kitty.ResendDropped: image_id=%d cells=(%d,%d)", id, up.cols, up.rows)
		if err := emitKittyUpload(w, id, up.payload, up.cols, up.rows); err != nil {
			return err
		}
		k.registry.markSent(id, up.decodedSize)
	}
	return nil
}

// lastResend is the frame and upload total ResendDroppedImages last
// checked: an unchanged frame with nothing sent since needs no scan.
var lastResend struct {
	frame string
	sent  uint64
}

// ResendDroppedImages runs ResendDropped for a finished frame against the
// process's kitty renderer and side channel. Called from the UI
// goroutine once per View.
func ResendDroppedImages(frame string) {
	if !strings.ContainsRune(frame, PlaceholderRune) {
		return
	}
	if frame == lastResend.frame && registry.sentBytes() == lastResend.sent {
		return
	}
	_ = kittyRendererInstance.ResendDropped(KittyOutput, frame)
	lastResend.frame, lastResend.sent = frame, registry.sentBytes()
}

// placedImageIDs lists, once each, the kitty image ids frame's
// placeholder cells reference: the foreground color in effect at each
// PlaceholderRune, read the way a terminal reads it, through every SGR
// sequence before it (a pill's background can sit between the
// placeholder's color and its cells).
func placedImageIDs(frame string) []uint32 {
	const ph = string(PlaceholderRune)
	var ids []uint32
	var fg uint32
	for rest := frame; ; {
		at := strings.Index(rest, ph)
		if at < 0 {
			return ids
		}
		// The SGR sequences between the last placeholder and this one.
		for seg := rest[:at]; ; {
			e := strings.Index(seg, "\x1b[")
			if e < 0 {
				break
			}
			end := e + 2
			for end < len(seg) && (seg[end] < 0x40 || seg[end] > 0x7e) {
				end++
			}
			if end == len(seg) {
				break
			}
			if seg[end] == 'm' {
				fg = sgrForeground(seg[e+2:end], fg)
			}
			seg = seg[end+1:]
		}
		if fg != 0 && !slices.Contains(ids, fg) {
			ids = append(ids, fg)
		}
		rest = rest[at+len(ph):]
	}
}

// sgrForeground applies one SGR parameter list to the foreground color
// fg, as the 24-bit value a placeholder cell's image id is read from,
// and returns it. 0 is no id: the default color or a basic one.
func sgrForeground(params string, fg uint32) uint32 {
	// Parsed by hand: this runs for every SGR sequence of every frame.
	var p [32]uint32 // zero past n, so p[i+1] reads 0 at the end
	n := 0
	var v uint32
	for j := 0; j <= len(params) && n < len(p)-1; j++ {
		if j == len(params) || params[j] == ';' {
			p[n] = v
			n++
			v = 0
		} else if c := params[j]; c >= '0' && c <= '9' {
			v = v*10 + uint32(c-'0')
		}
	}
	for i := 0; i < n; i++ {
		switch c := p[i]; {
		case c == 38 && p[i+1] == 2 && i+4 < n:
			fg = p[i+2]<<16 | p[i+3]<<8 | p[i+4]
			i += 4
		case c == 38 && p[i+1] == 5 && i+2 < n:
			fg = p[i+2]
			i += 2
		case (c == 48 || c == 58) && p[i+1] == 2:
			i += 4
		case (c == 48 || c == 58) && p[i+1] == 5:
			i += 2
		case c == 0 || c == 39 || (c >= 30 && c <= 37) || (c >= 90 && c <= 97):
			fg = 0
		}
	}
	return fg
}
