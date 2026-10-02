// internal/ui/copy_from_message.go
//
// Copying one piece out of a message: the `c` keybinding on the
// selected message offers its fenced code blocks and its links
// (messages.Copyables), and a click on the "copy" label a block wears
// on its top border copies that block. A block lands on the clipboard
// as drawn: no fence, no language tag, entities decoded, tabs kept. A
// link lands as its URL alone: no label, no angle brackets, and
// Slack's &amp; decoded so the URL works where it is pasted.
package ui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/linkpicker"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/statusbar"
)

// copyFromSelectedMessage implements the `c` keybinding on the selected
// message (messages pane or thread panel). 0 copyables -> toast; 1 ->
// copy it; 2+ -> open the picker modal in "copy" mode. Mirrors
// downloadFilesOfSelected.
func (a *App) copyFromSelectedMessage() tea.Cmd {
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
	copyables := messages.Copyables(msg)
	switch len(copyables) {
	case 0:
		return func() tea.Msg { return ToastMsg{Text: "Nothing to copy in message"} }
	case 1:
		return a.copyCopyable(copyables[0])
	default:
		items := make([]linkpicker.Item, len(copyables))
		links := make([]messages.Link, len(copyables))
		for i, c := range copyables {
			links[i] = c.Link
		}
		labels, whole := linkPickerLabels(links, a.plainMrkdwn)
		for i, c := range copyables {
			if c.Kind == messages.CopyableCodeBlock {
				items[i] = codeBlockPickerItem(c.CodeBlock)
				continue
			}
			items[i] = a.linkPickerItem(c.Link, labels[i], whole[i])
		}
		a.pickerKind = "copy"
		a.pickerCopyables = copyables
		a.linkPicker.Open("Copy from message", items)
		a.SetMode(ModeLinkPicker)
		return a.startLinkPreviews()
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

// linkPickerItem is a link's row in the `o`, `O` and `c` pickers: a
// permalink shows its decoded fallback text, and where it points in the
// Side column, until linkPreviewsInView fills an in-app one with its
// message. The
// filter reads whole, the label before linkPickerLabels cut it, and the
// permalink's channel, or the URL of a row that draws its URL: text
// that is known now and never changes.
func (a *App) linkPickerItem(l messages.Link, label, whole string) linkpicker.Item {
	item := linkpicker.Item{URL: l.URL, Label: label, FilterText: whole + " " + l.URL, InApp: a.linkOpensInApp(l.URL)}
	if pl, ok := slackurl.Parse(l.URL); ok {
		item.Display = permalinkRowText(pl)
		item.Side = a.permalinkPlace(pl, item.InApp)
		item.FilterText = whole + " " + item.Side
	}
	return item
}

// copyPickedCopyable is the copy picker's Enter.
func (a *App) copyPickedCopyable(index int) tea.Cmd {
	copyables := a.pickerCopyables
	a.pickerCopyables = nil
	a.pickerKind = ""
	if index < 0 || index >= len(copyables) {
		return nil
	}
	return a.copyCopyable(copyables[index])
}

func (a *App) copyCopyable(c messages.Copyable) tea.Cmd {
	if c.Kind == messages.CopyableCodeBlock {
		return a.copyCode(c.Text)
	}
	return tea.Batch(
		a.clipboardWrite(c.Text),
		func() tea.Msg { return ToastMsg{Text: "Copied link"} },
	)
}

func (a *App) copyCode(code string) tea.Cmd {
	n := len([]rune(code))
	return tea.Batch(
		a.clipboardWrite(code),
		func() tea.Msg { return statusbar.CopiedMsg{N: n} },
	)
}
