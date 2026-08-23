package image

import (
	"strconv"

	"github.com/gammons/slk/internal/debuglog"
)

// CellPixels returns the (width, height) of a terminal cell in pixels.
// It honors $COLORTERM_CELL_WIDTH/$COLORTERM_CELL_HEIGHT, then attempts
// TIOCGWINSZ on the given fd (unix only), then falls back to (8, 16).
// measured is false only on the fallback, so callers can tell "the
// terminal reported this" from "we guessed" and try another source
// (e.g. the XTWINOPS probe) before settling.
//
// fd is typically int(os.Stdout.Fd()). Pass -1 to skip the ioctl path.
// On Windows the ioctl path is unavailable; only the env override and
// fallback apply.
func CellPixels(fd int) (pxW, pxH int, measured bool) {
	if w, ok := atoi(getenv("COLORTERM_CELL_WIDTH")); ok {
		if h, ok := atoi(getenv("COLORTERM_CELL_HEIGHT")); ok {
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=env_override", w, h)
			return w, h, true
		}
	}
	if fd >= 0 {
		if w, h, ok := winsizePixels(fd); ok {
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=ioctl fd=%d", w, h, fd)
			return w, h, true
		}
	}
	debuglog.ImgRender("CellPixels: cell_w=8 cell_h=16 source=fallback (no env, no ioctl)")
	return 8, 16, false
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
