package thread

import "github.com/gammons/slk/internal/ui/messages"

// CodeBlockAt returns the code of the fenced block whose copy label is
// drawn at pane-local (y, x), the frame ClickAt takes. The parent's
// blocks count too.
func (m *Model) CodeBlockAt(y, x int) (code string, ok bool) {
	if y < m.chromeHeight {
		return "", false
	}
	line := y - m.chromeHeight + m.list.YOffset()
	entry, start, payload, ok := m.forkEntryAt(line)
	if !ok {
		return "", false
	}
	block, ok := messages.CodeBlockCopyLabelAt(payload.codeBlockCopyLabels, line-start, x)
	if !ok {
		return "", false
	}
	msg := m.parent
	if payload != &m.parentEntry {
		msg = m.replies[entry.Ref]
	}
	blocks := messages.CodeBlocks(msg)
	if block >= len(blocks) {
		return "", false
	}
	return blocks[block].Code, true
}
