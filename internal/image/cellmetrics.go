package image

import (
	"strconv"
	"sync/atomic"

	"github.com/gammons/slk/internal/debuglog"
)

// Default cell geometry, used until SetCellPixels reports the real
// values. Matches the CellPixels fallback so behaviour is unchanged on
// terminals that don't answer TIOCGWINSZ.
const (
	defaultCellPxW = 8
	defaultCellPxH = 16
)

// cellPxW / cellPxH hold the terminal's measured cell size in pixels —
// the resolution both pixel-addressed renderers encode against, for
// different reasons:
//
//   - sixel has no cell-aware placement: the terminal paints one sixel
//     pixel per device pixel and advances the cursor by the image's own
//     height, so the encoded pixel dimensions ARE the on-screen size and
//     must match the rows the layout reserved.
//   - kitty transmits c=<cols>,r=<rows> and stretches whatever it
//     receives across that cell box, so encoding at fewer pixels than
//     the box holds throws away detail the terminal cannot put back.
//
// Set once at startup from CellPixels() before any rendering goroutine
// runs; atomics keep that safe against the prerender worker pool.
var (
	cellPxW atomic.Int32
	cellPxH atomic.Int32
)

// SetCellPixels records the terminal's cell size for pixel-addressed
// encoding. Non-positive values are ignored, so a terminal that reports
// nothing keeps the 8x16 default. Call once during startup.
func SetCellPixels(w, h int) {
	if w <= 0 || h <= 0 {
		debuglog.ImgRender("SetCellPixels: ignoring non-positive cell size %dx%d", w, h)
		return
	}
	cellPxW.Store(int32(w))
	cellPxH.Store(int32(h))
	debuglog.ImgRender("SetCellPixels: cell_w=%d cell_h=%d", w, h)
}

// cellPixels returns the cell size to encode against.
func cellPixels() (int, int) {
	w, h := int(cellPxW.Load()), int(cellPxH.Load())
	if w <= 0 || h <= 0 {
		return defaultCellPxW, defaultCellPxH
	}
	return w, h
}

// CellPixels returns the (width, height) of a terminal cell in pixels.
// It honors $COLORTERM_CELL_WIDTH/$COLORTERM_CELL_HEIGHT, then attempts
// TIOCGWINSZ on the given fd (unix only), then falls back to (8, 16).
//
// fd is typically int(os.Stdout.Fd()). Pass -1 to skip the ioctl path.
// On Windows the ioctl path is unavailable; only the env override and
// fallback apply.
func CellPixels(fd int) (pxW, pxH int) {
	if w, ok := atoi(getenv("COLORTERM_CELL_WIDTH")); ok {
		if h, ok := atoi(getenv("COLORTERM_CELL_HEIGHT")); ok {
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=env_override", w, h)
			return w, h
		}
	}
	if fd >= 0 {
		if w, h, ok := winsizePixels(fd); ok {
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=ioctl fd=%d", w, h, fd)
			return w, h
		}
	}
	debuglog.ImgRender("CellPixels: cell_w=8 cell_h=16 source=fallback (no env, no ioctl)")
	return 8, 16
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
