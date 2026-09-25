// internal/ui/copy_code_block.go
//
// Copying one fenced code block out of a message: the `c` keybinding on
// the selected message, and a click on the "copy" label a block wears
// on its top border. Both resolve blocks through messages.CodeBlocks,
// so what lands on the clipboard is the block as drawn: no fence, no
// language tag, entities decoded, tabs kept.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/linkpicker"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
)

// copyCodeBlockOfSelected implements the `c` keybinding on the selected
// message (messages pane or thread panel). 0 blocks -> toast; 1 block
// -> copy it; 2+ -> open the picker modal in "code" mode. Mirrors
// downloadFilesOfSelected.
func (a *App) copyCodeBlockOfSelected() tea.Cmd {
	var msg messages.MessageItem
	switch a.focusedPanel {
	case PanelMessages:
		m, ok := a.messagepane.SelectedMessage()
		if !ok {
			return nil
		}
		msg = m
	case PanelThread:
		reply := a.threadPanel.SelectedReply()
		if reply == nil {
			return nil
		}
		msg = *reply
	default:
		return nil
	}
	blocks := messages.CodeBlocks(msg)
	switch len(blocks) {
	case 0:
		return func() tea.Msg { return ToastMsg{Text: "Message has no code block"} }
	case 1:
		return a.copyCode(blocks[0].Code)
	default:
		items := make([]linkpicker.Item, len(blocks))
		for i, b := range blocks {
			items[i] = codeBlockPickerItem(b)
		}
		a.pickerKind = "code"
		a.pickerCodeBlocks = blocks
		a.linkPicker.Open("Copy code block", items)
		a.SetMode(ModeLinkPicker)
		return nil
	}
}

// A row reads "<language>  <first line of code>  <N lines>".
func codeBlockPickerItem(b messages.CodeBlock) linkpicker.Item {
	lines := strings.Split(b.Code, "\n")
	item := linkpicker.Item{Label: b.Language, Detail: fmt.Sprintf("%d lines", len(lines))}
	if item.Label == "" {
		item.Label = "code"
	}
	if len(lines) == 1 {
		item.Detail = "1 line"
	}
	for _, line := range lines {
		if item.Display = strings.TrimSpace(line); item.Display != "" {
			break
		}
	}
	return item
}

// copyPickedCodeBlock is the code picker's Enter.
func (a *App) copyPickedCodeBlock(index int) tea.Cmd {
	blocks := a.pickerCodeBlocks
	a.pickerCodeBlocks = nil
	a.pickerKind = ""
	if index < 0 || index >= len(blocks) {
		return nil
	}
	return a.copyCode(blocks[index].Code)
}

func (a *App) copyCode(code string) tea.Cmd {
	n := len([]rune(code))
	return tea.Batch(
		a.clipboardWrite(code),
		func() tea.Msg { return statusbar.CopiedMsg{N: n} },
	)
}
