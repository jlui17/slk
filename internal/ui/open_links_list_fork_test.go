package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
	"github.com/gammons/slk/internal/usernames"
)

const listPermalink = "https://myteam.slack.com/archives/C054JFCBN69/p177928473300000"

// listLinksMessage is a bot post whose body is an ordered list of
// "<person> - <linked message>", then a paragraph with one labeled
// link. Its text is the body on one line, the labeled link as literal
// markdown, so the text has every URL and none of the structure.
func listLinksMessage() messages.MessageItem {
	item := func(user, n string) string {
		return `{"type":"rich_text_section","elements":[
			{"type":"user","user_id":"` + user + `"},
			{"type":"text","text":" - "},
			{"type":"message_mention","message_ts":"1.0","channel_id":"C054JFCBN69","url":"` + listPermalink + n + `"}]}`
	}
	return messages.MessageItem{
		TS: "1.0",
		Text: "Reviews waiting, oldest first: 1. <@U1> - <" + listPermalink + "1> 2. <@U2> - <" + listPermalink + "2> 3. <@U1> - <" + listPermalink + "3> " +
			"See also [the summary from Monday](<" + listPermalink + "9?thread_ts=1.0&amp;cid=C054JFCBN69>).",
		Blocks: blockkittest.FromJSON(`[{"type":"rich_text","elements":[
			{"type":"rich_text_section","elements":[{"type":"text","text":"Reviews waiting, oldest first:\n"}]},
			{"type":"rich_text_list","style":"ordered","indent":0,"offset":0,"elements":[` + item("U1", "1") + "," + item("U2", "2") + "," + item("U1", "3") + `]},
			{"type":"rich_text_section","elements":[
				{"type":"text","text":"See also "},
				{"type":"link","url":"` + listPermalink + `9?thread_ts=1.0&cid=C054JFCBN69","text":"the summary from Monday"},
				{"type":"text","text":"."}]}]},
			{"type":"context","elements":[{"type":"mrkdwn","text":"generated"}]}]`),
	}
}

func listLinksApp(t *testing.T) *App {
	t.Helper()
	app, _ := linkTestApp(t)
	app.SetHerdrTabOpener(func(url, label string, focus bool) error { return nil })
	app.SetUserNames(usernames.FromMap(map[string]string{"U1": "dana", "U2": "sam"}))
	app.width, app.height = 80, 24
	return app
}

func pickerLabels(app *App) (labels []string) {
	for _, it := range app.linkPicker.ItemsInView() {
		labels = append(labels, strings.TrimSpace(it.Label))
	}
	return labels
}

// A link in a list item is labeled with its item: the item's number and
// its text without the link. A labeled link in a paragraph shows its
// text, which the message's text fallback does not carry.
func TestLinkPicker_ListItemLinks(t *testing.T) {
	want := []string{"1. @dana", "2. @sam", "3. @dana", "the summary from Monday"}
	panes := map[string]func(*App){
		"messages pane": func(app *App) {
			app.focusedPanel = PanelMessages
			app.messagepane.SetMessages([]messages.MessageItem{listLinksMessage()})
		},
		"thread pane": func(app *App) {
			parent := messages.MessageItem{TS: "0.5", Text: "parent"}
			app.threadPanel.SetThread(parent, []messages.MessageItem{parent, listLinksMessage()}, "C1", "0.5")
			app.threadVisible = true
			app.focusedPanel = PanelThread
			for sel := app.threadPanel.SelectedReply(); sel == nil || sel.TS != "1.0"; sel = app.threadPanel.SelectedReply() {
				app.threadPanel.MoveDown()
			}
		},
	}
	for pane, show := range panes {
		for key, press := range map[string]func(*App) tea.Cmd{"o": pressO, "O": pressShiftO} {
			t.Run(pane+" "+key, func(t *testing.T) {
				app := listLinksApp(t)
				show(app)
				press(app)
				if got := pickerLabels(app); !slices.Equal(got, want) {
					t.Errorf("labels = %q, want %q", got, want)
				}
			})
		}
	}
}

func TestLinkPicker_ListItemLinks_FilterByPerson(t *testing.T) {
	app := listLinksApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{listLinksMessage()})
	pressShiftO(app)
	pickerScreen(t, app)
	pickerKeys(app, typed("/dana")...)
	pickerScreen(t, app)
	if got := pickerLabels(app); !slices.Equal(got, []string{"1. @dana", "3. @dana"}) {
		t.Errorf("filter \"dana\" shows %q, want dana's two rows", got)
	}
}

// Rows of other list shapes: a list that starts at 5, an item with two
// links ("item text · link text"), numbered items that are only a link
// (the number, then the link text when there is one), a bullet list and
// a nested item.
func TestLinkPicker_ListItemLinks_Shapes(t *testing.T) {
	link := func(n, text string) string {
		return `{"type":"link","url":"https://x.example/` + n + `","text":"` + text + `"}`
	}
	txt := func(s string) string { return `{"type":"text","text":"` + s + `"}` }
	item := func(elements ...string) string {
		return `{"type":"rich_text_section","elements":[` + strings.Join(elements, ",") + `]}`
	}
	list := func(style, indent, offset string, items ...string) string {
		return `{"type":"rich_text_list","style":"` + style + `","indent":` + indent + `,"offset":` + offset + `,"elements":[` + strings.Join(items, ",") + `]}`
	}
	app := listLinksApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{
		TS:   "1.0",
		Text: "<https://x.example/a|before> <https://x.example/b|after> <https://x.example/c|only a link> <https://x.example/m> <https://x.example/d|notes> <https://x.example/e|draft>",
		Blocks: blockkittest.FromJSON(`[{"type":"rich_text","elements":[` +
			list("ordered", "0", "4",
				item(txt("compare "), link("a", "before"), txt(" with "), link("b", "after")),
				item(link("c", "only a link")),
				item(`{"type":"message_mention","message_ts":"1.0","channel_id":"C1","url":"https://x.example/m"}`)) + "," +
			list("bullet", "0", "0", item(`{"type":"user","user_id":"U2"}`, txt(": "), link("d", "notes"))) + "," +
			list("bullet", "1", "0", item(txt("nested, "), link("e", "draft"))) + `]}]`),
	}})
	pressO(app)
	want := []string{"5. compare with · before", "5. compare with · after", "6. only a link", "7.", "@sam · notes", "nested · draft"}
	if got := pickerLabels(app); !slices.Equal(got, want) {
		t.Errorf("labels = %q, want %q", got, want)
	}
}
