package ui

import (
	"context"
	"regexp"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
)

// openLinksOfSelectedIn implements the `o`, `O` and `alt+o` keys, the
// fork's openLinksOfSelected: collect the links in the selected message
// (messages pane or thread panel). 0 links -> toast; 1 link -> dispatch
// OpenLinkMsg directly; 2+ -> open the link picker modal. All opens
// converge on OpenLinkMsg, the single routing point in reducer_links.go;
// inHerdr (the O and alt+o paths) rides along as OpenLinkMsg.InHerdr.
// The one exception: rows marked in the herdr picker open as
// OpenLinksInHerdrMsg.
func (a *App) openLinksOfSelectedIn(inHerdr HerdrTarget) tea.Cmd {
	p, _, ok := a.focusedContentPane()
	if !ok {
		return nil
	}
	selected, ok := p.SelectedMessage()
	if !ok {
		return nil
	}
	links := messages.MessageLinks(selected)
	hadLinks := len(links) > 0
	// When O or alt+o will actually open a herdr tab or pane, offer only
	// the links it can, and say so in the picker title: a browser
	// fallback under a title promising a tab is misleading. Outside herdr
	// (no opener) both degrade to o, full link list included.
	if a.herdrOpener(inHerdr) != nil {
		navigable := links[:0]
		for _, l := range links {
			if a.linkOpensInApp(l.URL) {
				navigable = append(navigable, l)
			}
		}
		links = navigable
	}
	switch len(links) {
	case 0:
		if hadLinks {
			return func() tea.Msg { return ToastMsg{Text: "No slk-openable links in message"} }
		}
		return func() tea.Msg { return ToastMsg{Text: "No links in message"} }
	case 1:
		url := links[0].URL
		return func() tea.Msg { return OpenLinkMsg{URL: url, InHerdr: inHerdr} }
	default:
		return a.openLinkPicker(links, inHerdr)
	}
}

// openLinkPicker is the arm of openLinksOfSelectedIn for two or more links.
func (a *App) openLinkPicker(links []messages.Link, inHerdr HerdrTarget) tea.Cmd {
	openerActive := a.herdrOpener(inHerdr) != nil
	items := a.linkPickerItems(links)
	title := "Open link"
	if openerActive {
		title = "Open link in herdr " + inHerdr.String()
	}
	a.pickerKind = "links"
	a.pickerInHerdr = inHerdr
	a.linkPicker.Open(title, items)
	a.linkPicker.SetMultiSelect(openerActive)
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
			labels[i] = cellwidth.Cut(text, room)
		}
		labels[i] = cellwidth.Cut(name, linkLabelWidth-lipgloss.Width(labels[i])) + labels[i]
		whole[i] = name + text
	}
	if !anyContext {
		return labels, whole
	}
	column := 0
	for i, label := range labels {
		labels[i] = cellwidth.Cut(label, linkLabelWidth)
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
// as a batch of herdr tabs or panes. While the filter is being typed,
// space and a are its text.
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
		target := a.pickerInHerdr
		a.linkPicker.Close()
		a.SetMode(ModeNormal)
		a.pickerInHerdr = HerdrNone
		return func() tea.Msg { return OpenLinksInHerdrMsg{URLs: urls, Target: target} }, true
	}
	return nil, false
}

// permalinkRowText is a permalink row's fallback text: what the picker
// shows until (or instead of, on fetch failure) the message preview,
// "Today · thread reply". Where the link points is the row's Side
// column, permalinkPlace.
func permalinkRowText(pl slackurl.Permalink) string {
	var parts []string
	if date := messages.DateFromTS(string(pl.MessageTS)); date != "" {
		parts = append(parts, messages.FormatDateSeparator(date))
	}
	if pl.ThreadTS != "" && string(pl.ThreadTS) != string(pl.MessageTS) {
		parts = append(parts, "thread reply")
	}
	return strings.Join(parts, " · ")
}

// permalinkPlace is where a permalink points: "#channel" for an in-app
// link, "sub.slack.com" for a foreign workspace.
func (a *App) permalinkPlace(pl slackurl.Permalink, inApp bool) string {
	if !inApp {
		return pl.Subdomain + ".slack.com"
	}
	// inApp implies the Lookup succeeds (linkOpensInApp requires it).
	name, chType, _ := a.channels.Lookup(pl.ChannelID)
	return channelDisplayName(name, chType)
}

func channelDisplayName(name, chType string) string {
	return messages.ChannelGlyph(chType) + name
}

// fetchLinkPreview resolves a picker row's target message via
// MessageService.Preview off the Update loop. Failures and unresolved
// messages produce no msg: the row keeps its permalinkRowText.
func (a *App) fetchLinkPreview(gen uint64, index int, pl slackurl.Permalink) tea.Cmd {
	messageSvc := a.messageSvc
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		sender, text, err := messageSvc.Preview(ctx, pl.ChannelID, pl.MessageTS, pl.ThreadTS)
		if err != nil {
			debuglog.General("linkPreview: %s/%s: %v", pl.ChannelID, pl.MessageTS, err)
			return nil
		}
		if text == "" {
			return nil
		}
		return LinkPreviewMsg{Index: index, Gen: gen, ChannelID: string(pl.ChannelID), Sender: sender, Text: text}
	}
}
