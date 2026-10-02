package ui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/linkpicker"
	"github.com/gammons/slk/internal/ui/messages"
)

// openLinkPicker is the arm of openLinksOfSelected for two or more links.
func (a *App) openLinkPicker(links []messages.Link, inHerdrTab, tabOpenerActive bool) tea.Cmd {
	labels, whole := linkPickerLabels(links, a.plainMrkdwn)
	items := make([]linkpicker.Item, len(links))
	for i, l := range links {
		items[i] = a.linkPickerItem(l, labels[i], whole[i])
	}
	title := "Open link"
	if tabOpenerActive {
		title = "Open link in herdr tab"
	}
	a.pickerKind = "links"
	a.pickerInTab = inHerdrTab
	a.linkPicker.Open(title, items)
	a.linkPicker.SetMultiSelect(tabOpenerActive)
	a.SetMode(ModeLinkPicker)
	return a.startLinkPreviews()
}

// startLinkPreviews is called once the picker is open on link rows.
func (a *App) startLinkPreviews() tea.Cmd {
	a.linkPreviewGen++
	a.linkPreviewAsked = map[int]bool{}
	return a.linkPreviewsInView()
}

// linkPreviewsInView fetches the message preview of each in-app
// permalink row the picker's scroll window shows and has not shown
// before. Nothing limits the Slack calls behind a preview that misses
// the cache, and a table can hold dozens of permalinks, so the rows are
// fetched as they come into view and not all at Open. It runs after
// Open, after each key and after a resize, before the draw that would
// tell the picker the terminal's height, so it tells it first.
func (a *App) linkPreviewsInView() tea.Cmd {
	a.linkPicker.SetTermHeight(a.height)
	var previews []tea.Cmd
	for _, it := range a.linkPicker.ItemsInView() {
		pl, ok := slackurl.Parse(it.URL)
		if !ok || !it.InApp || a.linkPreviewAsked[it.Index] {
			continue
		}
		a.linkPreviewAsked[it.Index] = true
		previews = append(previews, a.fetchLinkPreview(a.linkPreviewGen, it.Index, pl))
	}
	return tea.Batch(previews...)
}

// linkLabelWidth is the most a label column takes of a picker row.
const linkLabelWidth = 28

// linkPickerLabels is the whole layout of a link row's label. A link
// with a Context, its table row or its list item, reads "<context> ·
// <link text>", the context as plain text and either part absent when
// the link has none: the text alone is "1" or "2" in a table of numbered
// links, and nothing for a linked message in a list of them. When any
// link has a Context, every label is cut to linkLabelWidth and padded to
// one column, so the eye can run down it. A numbered list item that is
// only its link reads as the list draws it, "5. release notes", or "5."
// for a link without text. The context is cut before the link text,
// down to what leaves the text 10 cells. whole is each label before any
// cut, for the filter.
func linkPickerLabels(links []messages.Link, plain func(mrkdwn string) string) (labels, whole []string) {
	labels, whole = make([]string, len(links)), make([]string, len(links))
	anyContext := false
	for i, l := range links {
		if l.Label == l.URL {
			l.Label = "" // the row prints the URL itself
		}
		labels[i], whole[i] = l.Label, l.Label
		name, text := plain(l.Context), ""
		if name == "" {
			continue // no context, or one that draws as nothing
		}
		anyContext = true
		if l.Label != "" {
			text = " · " + l.Label
			if listNumberRe.MatchString(name) {
				text = " " + l.Label // "5. release notes", as the list draws it
			}
			room := max(linkLabelWidth-lipgloss.Width(name), lipgloss.Width(" · ")+10)
			labels[i] = linkpicker.Cut(text, room)
		}
		labels[i] = linkpicker.Cut(name, linkLabelWidth-lipgloss.Width(labels[i])) + labels[i]
		whole[i] = name + text
	}
	if !anyContext {
		return labels, whole
	}
	column := 0
	for i, label := range labels {
		labels[i] = linkpicker.Cut(label, linkLabelWidth)
		column = max(column, lipgloss.Width(labels[i]))
	}
	for i, label := range labels {
		labels[i] = label + strings.Repeat(" ", column-lipgloss.Width(label))
	}
	return labels, whole
}

// listNumberRe is the Context of a numbered list item that is nothing
// but its link.
var listNumberRe = regexp.MustCompile(`^\d+\.$`)

// plainMrkdwn is mrkdwn as one row of plain text: drawn as the threads
// list draws a parent preview, so a mention reads as its display name
// and an emoji as its glyph, then stripped of the styling.
func (a *App) plainMrkdwn(mrkdwn string) string {
	drawn := messages.RenderSlackMarkdownWith(mrkdwn, messages.RenderSlackMarkdownOpts{
		UserNames:    a.userNames.Current(),
		ChannelNames: a.channelNames,
		UserGroups:   a.userGroups,
		Preview:      true,
	})
	return strings.Join(strings.Fields(ansi.Strip(drawn)), " ")
}

// handleLinkPickerMarkKeys owns the keys a multi-select link picker
// adds: space marks the cursor row, a marks or clears all the rows its
// filter shows, and enter with anything marked opens the marked links
// as a batch of herdr tabs. While the filter is being typed, space and
// a are its text.
// Everything else (enter with nothing marked included) reports
// unhandled and takes handleLinkPickerMode's single-choice path.
func (a *App) handleLinkPickerMarkKeys(msg tea.KeyMsg) (tea.Cmd, bool) {
	if !a.linkPicker.MultiSelect() || a.linkPicker.Filtering() && msg.String() != "enter" {
		return nil, false
	}
	switch msg.String() {
	case "space":
		a.linkPicker.ToggleMark()
		return nil, true
	case "a":
		a.linkPicker.ToggleMarkAll()
		return nil, true
	case "enter":
		marked := a.linkPicker.Marked()
		if len(marked) == 0 {
			return nil, false
		}
		urls := make([]string, len(marked))
		for i, it := range marked {
			urls[i] = it.URL
		}
		a.linkPicker.Close()
		a.SetMode(ModeNormal)
		a.pickerInTab = false
		return func() tea.Msg { return OpenLinksInHerdrTabsMsg{URLs: urls} }, true
	}
	return nil, false
}
