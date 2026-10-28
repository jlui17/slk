package scrollbar

import "image/color"

// Cache remembers, per content line, the part of a row that Overlay
// keeps before the gutter glyph, so a pane that redraws the same rows
// frame after frame measures and cuts each row once instead of on every
// frame. A scroll moves the window over lines already cut.
//
// An entry is keyed by the line's index in the content (yOffset plus the
// row) and holds the row it was cut from. It is used only while that
// line still draws the same row, so content that changed under it costs
// a recompute, never a wrong frame. A width change drops every entry;
// rows Overlay pads are never cached, since the pad depends on bg.
//
// The zero value is ready to use.
type Cache struct {
	width int
	lines []cachedLine
}

type cachedLine struct {
	row, prefix string
	ok          bool
}

// Overlay is the package-level Overlay, reading and filling c.
// visible[i] must be line yOffset+i of the content.
func (c *Cache) Overlay(visible []string, width, total, yOffset, visibleHeight int, bg, trackFg, thumbFg color.Color) []string {
	return overlay(c, visible, width, total, yOffset, visibleHeight, bg, trackFg, thumbFg)
}

// Reset drops every entry. Callers call it when they replace the
// content, so the cache stops holding the old rows.
func (c *Cache) Reset() {
	clear(c.lines)
	c.lines = c.lines[:0]
}

func (c *Cache) setWidth(width int) {
	if c != nil && c.width != width {
		c.Reset()
		c.width = width
	}
}

func (c *Cache) get(line int, row string) (string, bool) {
	if c == nil || line < 0 || line >= len(c.lines) {
		return "", false
	}
	e := &c.lines[line]
	if !e.ok || e.row != row {
		return "", false
	}
	return e.prefix, true
}

func (c *Cache) put(line int, row, prefix string) {
	if c == nil || line < 0 {
		return
	}
	if line >= len(c.lines) {
		c.lines = append(c.lines, make([]cachedLine, line+1-len(c.lines))...)
	}
	c.lines[line] = cachedLine{row: row, prefix: prefix, ok: true}
}
