package image

import (
	"bytes"
	"image"
	"io"
	"strings"

	"github.com/gammons/slk/internal/debuglog"
	gosixel "github.com/mattn/go-sixel"
	"golang.org/x/image/draw"
)

// SixelRenderer encodes images as DEC sixel byte streams.
type SixelRenderer struct{}

// NewSixelRenderer returns a stateless sixel renderer.
func NewSixelRenderer() *SixelRenderer {
	return &SixelRenderer{}
}

// Render emits a Render whose Lines reserve the image's cell footprint
// with plain spaces, exactly like a text block of the same size — the
// messages pane treats them as ordinary content, reserves the rows, and
// drives the sixel byte stream from the placement pipeline (see
// internal/ui/messages and imgpkg.SixelPaint) rather than from the line
// content. Fallback carries the half-block equivalent used when the
// image is only partially visible.
func (s *SixelRenderer) Render(img image.Image, target image.Point) Render {
	if target.X <= 0 || target.Y <= 0 {
		debuglog.ImgRender("sixel.Render: target=(%d,%d) abort=zero_target", target.X, target.Y)
		return Render{Cells: target}
	}

	cw, ch := cellPixels()
	pxW := target.X * cw
	pxH := target.Y * ch
	resized := image.NewRGBA(image.Rect(0, 0, pxW, pxH))
	draw.BiLinear.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Over, nil)

	var sx bytes.Buffer
	enc := gosixel.NewEncoder(&sx)
	if err := enc.Encode(resized); err != nil {
		debuglog.ImgRender("sixel.Render: target=(%d,%d) encode_err=%v fallback=halfblock",
			target.X, target.Y, err)
		return HalfBlockRenderer{}.Render(img, target)
	}
	sixelBytes := sx.Bytes()
	debuglog.ImgRender("sixel.Render: target=(%d,%d) cell=(%d,%d) px=(%d,%d) sixel_bytes=%d",
		target.X, target.Y, cw, ch, pxW, pxH, len(sixelBytes))

	hb := HalfBlockRenderer{}.Render(img, target)

	lines := make([]string, target.Y)
	for i := range lines {
		lines[i] = strings.Repeat(" ", target.X)
	}

	bs := sixelBytes
	return Render{
		Cells:    target,
		Lines:    lines,
		Fallback: hb.Lines,
		OnFlush: func(w io.Writer) error {
			_, err := w.Write(bs)
			return err
		},
	}
}
