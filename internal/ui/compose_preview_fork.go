// internal/ui/compose_preview_fork.go
//
// Thumbnails of the images pasted into a compose box, drawn above its
// chip row. The preview is derived from the compose model's pending
// attachments on every frame, so removing an attachment or sending the
// message takes the thumbnail away with no state to invalidate.
//
// Only attachments that carry their bytes (a clipboard image) are
// previewed: a pasted file path would need a file read, which the TUI
// does not do itself.
//
// Selecting a thumbnail with the keyboard, to view or remove it, is in
// compose_preview_focus_fork.go.
//
// Sixel terminals get the half-block thumbnail. A sixel image is
// painted outside View by a painter that tracks its screen position,
// which is more than a thumbnail is worth for now.
package ui

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"golang.org/x/image/draw"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/compose"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

const (
	pastePreviewMaxRows = 8
	pastePreviewMaxCols = 32
	// pastePreviewPaneReserve is what the preview leaves of the pane's
	// content height besides the compose view: the two border edges,
	// the typing row, the spacer above the compose box, and five rows
	// of messages. A pane too short for a two-row preview shows none.
	pastePreviewPaneReserve = 2 + 1 + 1 + 5
)

// pasteThumbKey names a thumbnail by everything that decides its
// lines. data and n identify the pasted bytes by identity rather than
// by content hash: the bottom region is redrawn on every keystroke,
// and hashing megabytes of PNG per frame would cost more than the
// lookup saves. Holding the pointer keeps the address from being
// reused for other bytes while the entry lives.
type pasteThumbKey struct {
	data    *byte
	n       int
	maxRows int
	cell    image.Point
	proto   imgpkg.Protocol
}

// pasteThumb is one rendered thumbnail; lines is nil when the bytes
// did not decode as an image.
type pasteThumb struct {
	cols  int
	lines []string
}

// pastePreviewMemo is package-level because the thumbnails belong to
// the pasted bytes, not to an App, and a field on App would be churn
// on an upstream file. Only View touches it.
var pastePreviewMemo = map[pasteThumbKey]pasteThumb{}

// withPastePreview returns composeView with a row of thumbnails above
// it, one per pasted image, or composeView itself when there is
// nothing to preview. Every preview row is exactly width cells wide.
// contentHeight is the pane's content height, used to keep the preview
// from squeezing out the messages.
//
// Each thumbnail has a one-column gutter on its left, which lines the
// row up with the chips' indent and holds the selection marker, so
// selecting moves nothing. The marker and the hint are built here on
// every frame; only the thumbnail lines come from the memo.
func (a *App) withPastePreview(m *compose.Model, composeView string, width, contentHeight int) string {
	a.forgetUnheldPasteThumbs()
	maxRows := min(pastePreviewMaxRows, contentHeight-lipgloss.Height(composeView)-pastePreviewPaneReserve)

	var thumbs []pasteThumb
	var drawn []int // the attachment each thumbnail belongs to
	used, height := 0, 0
	if a.imgProtocol != imgpkg.ProtoOff && maxRows >= 2 {
		for i, att := range m.Attachments() {
			if len(att.Bytes) == 0 {
				continue
			}
			t := a.pasteThumb(att.Bytes, maxRows)
			if t.lines == nil {
				continue
			}
			if used+1+t.cols > width {
				break
			}
			thumbs = append(thumbs, t)
			drawn = append(drawn, i)
			used += 1 + t.cols
			height = max(height, len(t.lines))
		}
	}
	if a.pasteThumbFocus.drawn == nil {
		a.pasteThumbFocus.drawn = map[*compose.Model][]int{}
	}
	a.pasteThumbFocus.drawn[m] = drawn
	selected := a.selectedPasteThumb(m)
	if len(thumbs) == 0 {
		return composeView
	}

	hint := ""
	switch {
	case selected >= 0:
		hint = "enter view · ⌫ remove · esc back"
	case a.pasteThumbRowReachable(m):
		hint = "↑ select"
	}
	if used+2+lipgloss.Width(hint) > width {
		hint = ""
	}
	// The marker reads by its shape; the accent colour only matches
	// the other selected-item markers.
	marker := lipgloss.NewStyle().Foreground(styles.Accent)

	rows := make([]string, height)
	for r := range rows {
		var b strings.Builder
		for i, t := range thumbs {
			switch {
			case i != selected || r >= len(t.lines):
				b.WriteByte(' ')
			case r == 0:
				b.WriteString(marker.Render("▸"))
			default:
				b.WriteString(marker.Render("│"))
			}
			if r < len(t.lines) {
				b.WriteString(t.lines[r])
			} else {
				b.WriteString(strings.Repeat(" ", t.cols))
			}
		}
		cells := used
		if r == height-1 && hint != "" {
			b.WriteString("  " + lipgloss.NewStyle().Foreground(styles.TextMuted).Render(hint))
			cells += 2 + lipgloss.Width(hint)
		}
		b.WriteString(strings.Repeat(" ", width-cells))
		rows[r] = b.String()
	}
	// The pane background goes on as a prefix and after resets only:
	// kitty placeholder cells carry their image id in the foreground.
	return messages.WithBackground(rows, messages.BgANSI()) + "\n" + composeView
}

// pasteThumb decodes and renders data once per (bytes, size, protocol).
func (a *App) pasteThumb(data []byte, maxRows int) pasteThumb {
	cell := a.imageCtx.CellPixels
	if cell.X <= 0 || cell.Y <= 0 {
		cell = image.Pt(8, 16)
	}
	key := pasteThumbKey{data: &data[0], n: len(data), maxRows: maxRows, cell: cell, proto: a.imgProtocol}
	if t, ok := pastePreviewMemo[key]; ok {
		return t
	}
	var t pasteThumb
	if img, _, err := image.Decode(bytes.NewReader(data)); err == nil {
		target := pasteThumbCells(img.Bounds().Size(), cell, maxRows)
		t = pasteThumb{cols: target.X, lines: renderPasteThumb(a.imgProtocol, img, data, target, cell)}
	}
	pastePreviewMemo[key] = t
	return t
}

// renderPasteThumb draws img at target cells. Kitty goes through a
// content-hash key, not RenderImage: its anonymous key is derived from
// the image bounds, so two screenshots of one size would share an id.
// The kitty source is shrunk first because kitty sources are never
// evicted, and a full-size screenshot would stay in memory for the
// session.
func renderPasteThumb(proto imgpkg.Protocol, img image.Image, data []byte, target, cell image.Point) []string {
	if proto != imgpkg.ProtoKitty {
		return imgpkg.RenderImage(imgpkg.ProtoHalfBlock, img, target).Lines
	}
	small := image.NewRGBA(image.Rect(0, 0, target.X*cell.X, target.Y*cell.Y))
	draw.BiLinear.Scale(small, small.Bounds(), img, img.Bounds(), draw.Over, nil)
	sum := sha1.Sum(data)
	kittyKey := "paste-" + hex.EncodeToString(sum[:])
	imgpkg.KittyRendererInstance().SetSource(kittyKey, small)
	r := imgpkg.KittyRendererInstance().RenderKey(kittyKey, target)
	if r.OnFlush != nil {
		_ = r.OnFlush(imgpkg.KittyOutput)
	}
	return r.Lines
}

// pasteThumbCells fits a px-sized image into pastePreviewMaxCols x
// maxRows cells, keeping its aspect ratio on the terminal's cell and
// never drawing it larger than its own pixels.
func pasteThumbCells(px, cell image.Point, maxRows int) image.Point {
	if px.X <= 0 || px.Y <= 0 {
		return image.Point{}
	}
	maxCols := min(pastePreviewMaxCols, max(1, px.X/cell.X))
	rows := min(maxRows, max(1, px.Y/cell.Y))
	// cols / rows == (px.X / cell.X) / (px.Y / cell.Y)
	cols := rows * px.X * cell.Y / (px.Y * cell.X)
	if cols > maxCols {
		cols = maxCols
		rows = cols * px.Y * cell.X / (px.X * cell.Y)
	}
	return image.Pt(max(1, cols), max(1, rows))
}

// forgetUnheldPasteThumbs drops the thumbnails of bytes neither compose
// box holds any more, which also lets go of the bytes themselves.
func (a *App) forgetUnheldPasteThumbs() {
	if len(pastePreviewMemo) == 0 {
		return
	}
	held := map[*byte]bool{}
	for _, m := range []*compose.Model{&a.compose, &a.threadCompose} {
		for _, att := range m.Attachments() {
			if len(att.Bytes) > 0 {
				held[&att.Bytes[0]] = true
			}
		}
	}
	for k := range pastePreviewMemo {
		if !held[k.data] {
			delete(pastePreviewMemo, k)
		}
	}
}
