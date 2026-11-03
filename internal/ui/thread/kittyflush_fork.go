package thread

import (
	"bytes"
	"io"

	"github.com/gammons/slk/internal/bubbles/entrylist"
	imgpkg "github.com/gammons/slk/internal/image"
)

// flushVisible writes the kitty uploads of the parent and replies frame
// shows, straight to the terminal output (APC sequences embedded in line
// content get mangled by the renderer), as the channel pane does. A
// flush sends its image only when the terminal lacks it, so a frame
// that shows nothing new writes nothing. Uploading every reply instead
// overflowed herdr's image store on an image-heavy thread: it dropped
// the first uploads, slk then counted every image as dropped and sent
// the whole thread again on each frame.
func (m *Model) flushVisible(frame *entrylist.Frame) {
	entries := m.list.Entries()
	var buf bytes.Buffer
	for _, v := range frame.Visible {
		e := entries[v.Index]
		var flushes []func(io.Writer) error
		switch {
		case !e.Selectable: // a divider, rule or landmark
		case e.Ref == parentSelected:
			flushes = m.memo.flushes
		case e.Ref < len(m.cache):
			flushes = m.cache[e.Ref].Flushes
		}
		for _, fl := range flushes {
			if fl != nil {
				_ = fl(&buf)
			}
		}
	}
	if buf.Len() > 0 {
		_, _ = imgpkg.KittyOutput.Write(buf.Bytes())
	}
}
