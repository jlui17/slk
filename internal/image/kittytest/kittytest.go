// Package kittytest provides a stand-in for a kitty-graphics terminal
// whose image store is bounded, for tests of what slk uploads.
package kittytest

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/png" // uploads are f=100 (PNG)
	"strconv"
	"strings"
	"unicode/utf8"

	imgpkg "github.com/gammons/slk/internal/image"
)

// HerdrStoreBytes is herdr's (0.9.x) per-pane image store, in decoded
// bytes.
const HerdrStoreBytes = 64 << 20

// Terminal receives the kitty APC sequences slk writes and keeps the
// uploaded images the way herdr does: up to Cap decoded bytes, dropping
// the oldest upload first once a new one passes Cap, even one still on
// screen. An a=d,d=I deletes the image. Every Write must hold whole
// sequences, which emitKittyUpload guarantees for an upload.
type Terminal struct {
	Cap int

	// Uploads and UploadedBytes count every a=T upload and its decoded
	// size; Written counts the raw bytes written.
	Uploads       int
	UploadedBytes int
	Written       int
	// IDs lists the image id of every upload, in order.
	IDs    []uint32
	framed int // len(IDs) at the last Frame

	held   map[uint32]int // image id -> decoded bytes
	order  []uint32       // held ids, oldest upload first
	stored int

	pending   uint32 // id of the upload whose chunks are arriving
	payload   strings.Builder
	uploading bool
}

// New returns a Terminal holding at most limit decoded bytes.
func New(limit int) *Terminal {
	return &Terminal{Cap: limit, held: map[uint32]int{}}
}

func (t *Terminal) Write(p []byte) (int, error) {
	t.Written += len(p)
	rest := p
	for {
		start := bytes.Index(rest, []byte("\x1b_G"))
		if start < 0 {
			return len(p), nil
		}
		rest = rest[start+3:]
		end := bytes.Index(rest, []byte("\x1b\\"))
		if end < 0 {
			return len(p), nil
		}
		t.apc(string(rest[:end]))
		rest = rest[end+2:]
	}
}

func (t *Terminal) apc(body string) {
	ctrl, data, _ := strings.Cut(body, ";")
	keys := map[string]string{}
	for _, kv := range strings.Split(ctrl, ",") {
		k, v, _ := strings.Cut(kv, "=")
		keys[k] = v
	}
	switch {
	case keys["a"] == "d" && keys["d"] == "I":
		t.drop(parseID(keys["i"]))
		return
	case keys["a"] == "T":
		t.pending, t.uploading = parseID(keys["i"]), true
		t.payload.Reset()
	case !t.uploading:
		return
	}
	t.payload.WriteString(data)
	if keys["m"] == "1" {
		return
	}
	t.uploading = false
	png, err := base64.StdEncoding.DecodeString(t.payload.String())
	if err != nil {
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(png))
	if err != nil {
		return
	}
	t.store(t.pending, cfg.Width*cfg.Height*4)
}

func (t *Terminal) store(id uint32, decoded int) {
	t.Uploads++
	t.UploadedBytes += decoded
	t.IDs = append(t.IDs, id)
	t.drop(id)
	t.held[id] = decoded
	t.order = append(t.order, id)
	t.stored += decoded
	for t.stored > t.Cap && len(t.order) > 1 {
		t.drop(t.order[0])
	}
}

func (t *Terminal) drop(id uint32) {
	size, ok := t.held[id]
	if !ok {
		return
	}
	delete(t.held, id)
	t.stored -= size
	for i, o := range t.order {
		if o == id {
			t.order = append(t.order[:i], t.order[i+1:]...)
			break
		}
	}
}

// Holds reports whether the terminal still has image id.
func (t *Terminal) Holds(id uint32) bool {
	_, ok := t.held[id]
	return ok
}

// Frame checks a frame drawn after the uploads written since the last
// Frame: blank lists the images it places that the terminal does not
// hold (cells that draw nothing), offscreen the uploads of images it
// does not place.
func (t *Terminal) Frame(frame string) (placed map[uint32]bool, blank, offscreen []uint32) {
	placed = PlacedIDs(frame)
	for id := range placed {
		if !t.Holds(id) {
			blank = append(blank, id)
		}
	}
	for _, id := range t.IDs[t.framed:] {
		if !placed[id] {
			offscreen = append(offscreen, id)
		}
	}
	t.framed = len(t.IDs)
	return placed, blank, offscreen
}

// Stored is the decoded bytes the terminal holds.
func (t *Terminal) Stored() int { return t.stored }

func parseID(s string) uint32 {
	n, _ := strconv.ParseUint(s, 10, 32)
	return uint32(n)
}

// PlacedIDs returns the image ids a frame's kitty placeholder cells
// draw: each placeholder rune's id is its foreground color, set by
// 38;2;r;g;b (id r<<16|g<<8|b) or 38;5;n.
func PlacedIDs(frame string) map[uint32]bool {
	ids := map[uint32]bool{}
	var fg uint32
	for i := 0; i < len(frame); {
		if frame[i] == 0x1b && i+1 < len(frame) {
			switch frame[i+1] {
			case '[':
				j := i + 2
				for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
					j++
				}
				if j < len(frame) && frame[j] == 'm' {
					fg = sgrForeground(frame[i+2:j], fg)
				}
				i = j + 1
				continue
			case ']', '_', 'P':
				// OSC, APC, DCS: skip to BEL or ST.
				j := i + 2
				for j < len(frame) && frame[j] != 0x07 && !(frame[j] == 0x1b && j+1 < len(frame) && frame[j+1] == '\\') {
					j++
				}
				if j < len(frame) && frame[j] == 0x1b {
					j++
				}
				i = j + 1
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(frame[i:])
		if r == imgpkg.PlaceholderRune && fg != 0 {
			ids[fg] = true
		}
		i += size
	}
	return ids
}

// sgrForeground applies an SGR parameter list to the current foreground.
func sgrForeground(params string, fg uint32) uint32 {
	ps := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	if len(ps) == 0 {
		return 0
	}
	for i := 0; i < len(ps); i++ {
		n, _ := strconv.Atoi(ps[i])
		switch {
		case n == 0 || n == 39:
			fg = 0
		case (n == 38 || n == 48 || n == 58) && i+1 < len(ps):
			mode, _ := strconv.Atoi(ps[i+1])
			switch {
			case mode == 2 && i+4 < len(ps):
				if n == 38 {
					r, _ := strconv.Atoi(ps[i+2])
					g, _ := strconv.Atoi(ps[i+3])
					b, _ := strconv.Atoi(ps[i+4])
					fg = uint32(r)<<16 | uint32(g)<<8 | uint32(b)
				}
				i += 4
			case mode == 5 && i+2 < len(ps):
				if n == 38 {
					c, _ := strconv.Atoi(ps[i+2])
					fg = uint32(c)
				}
				i += 2
			}
		case n >= 30 && n <= 37, n >= 90 && n <= 97:
			fg = 0 // a palette color is no image id
		}
	}
	return fg
}
