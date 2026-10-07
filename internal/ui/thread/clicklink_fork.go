package thread

import "github.com/gammons/slk/internal/ui/messages"

// LinkURLAt returns the URL of the hyperlink drawn at pane-local
// (viewportY, x), the frame BeginSelectionAt takes, or "" when no link
// is drawn there.
func (m *Model) LinkURLAt(viewportY, x int) string {
	if viewportY < m.chromeHeight {
		return ""
	}
	abs := viewportY - m.chromeHeight + m.vp.YOffset()
	entry, start, _, ok := m.selectionEntryAt(abs)
	if !ok {
		return ""
	}
	return messages.LinkAt(entry.linesNormal[abs-start], x)
}
