package thread

import "github.com/gammons/slk/internal/ui/messages"

func (m *Model) newBodyCopyLabels() *[]messages.CodeBlockCopyLabel {
	m.bodyCopyLabels = nil
	return &m.bodyCopyLabels
}

// The body starts under the header row, right of the selection border.
func (m *Model) takeBodyCopyLabels() []messages.CodeBlockCopyLabel {
	labels := messages.OffsetCodeBlockCopyLabels(m.bodyCopyLabels, 1, 1)
	m.bodyCopyLabels = nil
	return labels
}

// CodeBlockAt returns the code of the fenced block whose copy label is
// drawn at pane-local (y, x), the frame ClickAt takes. The parent's
// blocks count too.
func (m *Model) CodeBlockAt(y, x int) (code string, ok bool) {
	if y < m.chromeHeight {
		return "", false
	}
	line := y - m.chromeHeight + m.vp.YOffset()
	entry, start, _, ok := m.selectionEntryAt(line)
	if !ok {
		return "", false
	}
	block, ok := messages.CodeBlockCopyLabelAt(entry.codeBlockCopyLabels, line-start, x)
	if !ok {
		return "", false
	}
	msg := m.parent
	if entry != &m.parentEntry {
		msg = m.replies[entry.replyIdx]
	}
	blocks := messages.CodeBlocks(msg)
	if block >= len(blocks) {
		return "", false
	}
	return blocks[block].Code, true
}
