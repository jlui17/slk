package ui

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/usernames"
)

func tablePermalink(n int) string {
	return fmt.Sprintf("https://myteam.slack.com/archives/C054JFCBN69/p17792847330%05d", n)
}

// tableLinksMessage is a bot post: a prose body with one permalink, then
// a table whose third column links to messages as "1", "2".
func tableLinksMessage() messages.MessageItem {
	cell := func(ns ...int) string {
		s := ""
		for i, n := range ns {
			if i > 0 {
				s += ", "
			}
			s += fmt.Sprintf("<%s|%d>", tablePermalink(n), i+1)
		}
		return s
	}
	return messages.MessageItem{
		TS:   "1.0",
		Text: "summary of <" + tablePermalink(0) + ">",
		Blocks: []blockkit.Block{
			blockkit.TableBlock{Rows: [][]string{
				{"Task", "Owner", "Notes", "Status"},
				{"job 1 (Q4)", "Dana", cell(1, 2), "A"},
				{"job 2 (Q5), Sam and Dana pairing", "Sam", cell(3), "B"},
			}},
			blockkit.ContextBlock{Elements: []blockkit.ContextElement{{Text: "generated"}}},
		},
	}
}

func wantTableLinks(t *testing.T, app *App) {
	t.Helper()
	if app.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want ModeLinkPicker", app.mode)
	}
	items := app.linkPicker.Items()
	if len(items) != 4 {
		t.Fatalf("picker has %d rows, want 4: %#v", len(items), items)
	}
	for i, it := range items {
		if it.URL != tablePermalink(i) {
			t.Errorf("row %d URL = %q, want %q", i, it.URL, tablePermalink(i))
		}
	}
}

func TestOpenLinks_TableLinks_MessagesPane(t *testing.T) {
	for name, press := range map[string]func(*App) tea.Cmd{"o": pressO, "O": pressShiftO} {
		t.Run(name, func(t *testing.T) {
			app, _ := linkTestApp(t)
			app.SetHerdrTabOpener(func(url, label string, focus bool) error { return nil })
			app.focusedPanel = PanelMessages
			app.messagepane.SetMessages([]messages.MessageItem{tableLinksMessage()})
			press(app)
			wantTableLinks(t, app)
		})
	}
}

func TestOpenLinks_TableLinks_ThreadPane(t *testing.T) {
	for name, press := range map[string]func(*App) tea.Cmd{"o": pressO, "O": pressShiftO} {
		t.Run(name, func(t *testing.T) {
			app, _ := linkTestApp(t)
			app.SetHerdrTabOpener(func(url, label string, focus bool) error { return nil })
			parent := messages.MessageItem{TS: "0.5", Text: "parent"}
			reply := tableLinksMessage()
			app.threadPanel.SetThread(parent, []messages.MessageItem{parent, reply}, "C1", "0.5")
			app.threadVisible = true
			app.focusedPanel = PanelThread
			for sel := app.threadPanel.SelectedReply(); sel == nil || sel.TS != reply.TS; sel = app.threadPanel.SelectedReply() {
				app.threadPanel.MoveDown()
			}
			press(app)
			wantTableLinks(t, app)
		})
	}
}

func TestLinkPickerLabels(t *testing.T) {
	plain := func(mrkdwn string) string { return strings.ToLower(mrkdwn) }
	got, whole := linkPickerLabels([]messages.Link{
		{URL: "https://a.example"},
		{URL: "https://b.example", Label: "1", Context: "Job 1 (Q4)"},
		{URL: "https://c.example", Label: "2", Context: "Job 2 (Q5), Sam and Dana pairing"},
		{URL: "https://d.example", Context: "Row 3"},
	}, plain)
	want := []string{
		"                            ",
		"job 1 (q4) · 1              ",
		"job 2 (q5), sam and dan… · 2",
		"row 3                       ",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d = %q, want %q", i, got[i], want[i])
		}
	}
	if whole[2] != "job 2 (q5), sam and dana pairing · 2" {
		t.Errorf("whole label = %q, want it uncut", whole[2])
	}

	asIs, _ := linkPickerLabels([]messages.Link{{URL: "https://a.example", Label: "docs"}, {URL: "https://b.example"}}, plain)
	if asIs[0] != "docs" || asIs[1] != "" {
		t.Errorf("labels outside a table = %q, want them as they are", asIs)
	}
}

// bigTableApp has O's picker open, on an 80x24 terminal, over a message
// with one permalink in its text and 45 in a 30-row table.
func bigTableApp(t *testing.T) (*App, tea.Cmd) {
	t.Helper()
	return bigTableAppSized(t, 80, 24)
}

// The preview of the table's second link is the short "ok".
func bigTableAppSized(t *testing.T, width, height int) (*App, tea.Cmd) {
	t.Helper()
	app, _ := linkTestApp(t)
	app.SetHerdrTabOpener(func(url, label string, focus bool) error { return nil })
	app.SetUserNames(usernames.FromMap(map[string]string{"U1": "dana"}))
	app.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Preview: func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			if ts == "1779284733.000002" {
				return "dana", "ok", nil
			}
			return "dana", "the nightly export skipped two regions again", nil
		},
	}))
	rows := [][]string{{"Task", "Owner", "Notes", "Status"}}
	names := []string{"job %d (Q%d)", "job %d (Q%d), Sam and Dana pairing"}
	for r, n := 1, 1; r <= 30; r++ {
		var cell []string
		for i := 1; i <= 1+r%2; i++ {
			cell = append(cell, fmt.Sprintf("<%s|%d>", tablePermalink(n), i))
			n++
		}
		rows = append(rows, []string{fmt.Sprintf(names[r%3/2], r, r+3), "Dana", strings.Join(cell, ", "), "A"})
	}
	app.width, app.height = width, height
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{
		TS:     "1.0",
		Text:   "summary of <" + tablePermalink(0) + ">",
		Blocks: []blockkit.Block{blockkit.TableBlock{Rows: rows}},
	}})
	cmd := pressShiftO(app)
	if n := len(app.linkPicker.Items()); n != 46 {
		t.Fatalf("picker has %d rows, want 46", n)
	}
	return app, cmd
}

func pickerScreen(t *testing.T, app *App) string {
	t.Helper()
	blank := strings.TrimSuffix(strings.Repeat(strings.Repeat(" ", app.width)+"\n", app.height), "\n")
	out := ansi.Strip(app.linkPicker.ViewOverlay(app.width, app.height, blank))
	t.Logf("\n%s", out)
	if n := strings.Count(out, "\n") + 1; n != app.height {
		t.Errorf("screen is %d lines, want %d", n, app.height)
	}
	return out
}

func pickerKeys(app *App, keys ...tea.KeyPressMsg) (cmds []tea.Cmd) {
	for _, k := range keys {
		cmds = append(cmds, app.handleKey(k))
	}
	return cmds
}

func typed(s string) (keys []tea.KeyPressMsg) {
	for _, r := range s {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

// The picker asks for the previews of the rows in view, not all 46, and
// for each further row as it scrolls into view.
func TestLinkPicker_BigTable_PreviewsFollowTheWindow(t *testing.T) {
	app, cmd := bigTableApp(t)
	opened := drainCmd(cmd)
	if len(opened) != 14 {
		t.Fatalf("opening fetched %d previews, want the 14 rows in view", len(opened))
	}
	for _, m := range opened {
		app.Update(m)
	}
	out := pickerScreen(t, app)
	for _, want := range []string{
		"1–14 of 46",
		"▌[ ]                               the nightly export skipped two r… [slk]",
		" [ ] job 1 (Q4) · 1                the nightly export skipped two r… [slk]",
		" [ ] job 5 (Q8), Sam and Dan… · 2  the nightly export skipped two r… [slk]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q", want)
		}
	}

	var scrolled []tea.Msg
	for _, c := range pickerKeys(app, typed(strings.Repeat("j", 30))...) {
		scrolled = append(scrolled, drainCmd(c)...)
	}
	if len(scrolled) != 17 {
		t.Errorf("30 downs fetched %d previews, want 17: one per row that came into view", len(scrolled))
	}
	for _, m := range scrolled {
		app.Update(m)
	}
	if out := pickerScreen(t, app); !strings.Contains(out, "18–31 of 46") || !strings.Contains(out, "▌[ ] job 20 (Q23), Sam and D… · 1") {
		t.Error("after 30 downs the window is not rows 18 to 31 with row 31 highlighted")
	}
}

// Filter to a table row, hand the keys back to the list, mark what the
// filter shows, open: the marks are the filtered rows only.
func TestLinkPicker_BigTable_FilterMarkAllOpen(t *testing.T) {
	app, _ := bigTableApp(t)
	pickerKeys(app, typed("/5 (q8), sa")...) // "a" and " " are filter text here, not mark keys
	if got := len(app.linkPicker.Marked()); got != 0 {
		t.Fatalf("%d rows marked while typing a filter", got)
	}
	out := pickerScreen(t, app)
	if !strings.Contains(out, "/5 (q8), sa█") || !strings.Contains(out, "2 matches") {
		t.Error("screen missing the filter line or the match count")
	}
	pickerKeys(app, tea.KeyPressMsg{Code: tea.KeyTab})
	pickerKeys(app, typed("a")...)
	cmds := pickerKeys(app, tea.KeyPressMsg{Code: tea.KeyEnter})
	msg, ok := cmds[0]().(OpenLinksInHerdrTabsMsg)
	if !ok || !slices.Equal(msg.URLs, []string{tablePermalink(7), tablePermalink(8)}) {
		t.Errorf("enter = %#v, want row 5's two links opened as herdr tabs", msg)
	}
	if app.mode != ModeNormal {
		t.Errorf("mode = %v, want ModeNormal", app.mode)
	}
}

// The filter reads the whole first cell, not the label as "…" cuts it,
// and the channel; a preview is not read, in view or not.
func TestLinkPicker_BigTable_FilterReadsPastTheCut(t *testing.T) {
	app, cmd := bigTableApp(t)
	for _, m := range drainCmd(cmd) {
		app.Update(m)
	}
	pickerKeys(app, typed("/pairing")...)
	out := pickerScreen(t, app)
	if !strings.Contains(out, "1–14 of 15 matches") || !strings.Contains(out, "▌[ ] job 2 (Q5), Sam and Dan… · 1") {
		t.Error("\"pairing\", cut from the label by …, did not keep the 15 links of the rows named with it")
	}
	pickerKeys(app, tea.KeyPressMsg{Code: tea.KeyEscape})
	pickerKeys(app, typed("/two regions")...)
	if out := pickerScreen(t, app); !strings.Contains(out, "no matching rows") {
		t.Error("\"two regions\" is in the 14 previews that landed and should match no row: previews are not read")
	}
	pickerKeys(app, tea.KeyPressMsg{Code: tea.KeyEscape})
	pickerKeys(app, typed("/#GENERAL")...)
	if out := pickerScreen(t, app); !strings.Contains(out, "1–14 of 46 matches") {
		t.Error("\"#GENERAL\" did not keep all 46 rows by their channel")
	}
}

// A table row is named in plain text: a mention by its display name,
// no * or ` markers, and the filter finds what the label shows.
func TestLinkPicker_ContextNameIsPlainText(t *testing.T) {
	app, _ := linkTestApp(t)
	app.SetUserNames(usernames.FromMap(map[string]string{"U1": "dana"}))
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{
		TS: "1.0",
		Blocks: []blockkit.Block{blockkit.TableBlock{Rows: [][]string{
			{"Task", "Notes"},
			{"<@U1> *bold* `code`", "<https://a.example|1>, <https://b.example|2>"},
			{"row 2", "<https://c.example|1>"},
		}}},
	}})
	pressO(app)
	items := app.linkPicker.Items()
	if len(items) != 3 || strings.TrimSpace(items[0].Label) != "@dana bold code · 1" {
		t.Fatalf("items = %#v, want the first labeled \"@dana bold code · 1\"", items)
	}
	pickerKeys(app, typed("/@dana b")...)
	if got := app.linkPicker.ItemsInView(); len(got) != 2 || got[1].URL != "https://b.example" {
		t.Errorf("filter \"@dana b\" shows %#v, want the first row's two links", got)
	}
}

// Long link text is cut too: every label of a table message is the
// column's width, however long its row name, its link text or the label
// of a link in the prose.
func TestLinkPickerLabels_LongLinkText(t *testing.T) {
	got, whole := linkPickerLabels([]messages.Link{
		{URL: "https://a.example", Label: "the design review notes for the build cache"},
		{URL: "https://b.example", Label: "notes from the design review", Context: "row 4, Sam: rename the build cache"},
		{URL: "https://c.example", Label: "notes from the design review", Context: "row 5"},
	}, func(mrkdwn string) string { return mrkdwn })
	want := []string{
		"the design review notes for…",
		"row 4, Sam: re… · notes fro…",
		"row 5 · notes from the desi…",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label %d = %q (%d cells), want %q", i, got[i], lipgloss.Width(got[i]), want[i])
		}
	}
	if whole[1] != "row 4, Sam: rename the build cache · notes from the design review" {
		t.Errorf("whole label = %q, want it uncut", whole[1])
	}
}

// A link whose text is its own URL prints the URL once, in a table
// message too, where its label is padded to the column.
func TestLinkPicker_TableMessage_URLAsLinkTextPrintsOnce(t *testing.T) {
	app, _ := linkTestApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{{
		TS:   "1.0",
		Text: "see <https://a.example/docs|https://a.example/docs>",
		Blocks: []blockkit.Block{blockkit.TableBlock{Rows: [][]string{
			{"Task", "Notes"},
			{"row 1", "<https://b.example|1>, <https://c.example/x|https://c.example/x>"},
		}}},
	}})
	pressO(app)
	out := ansi.Strip(app.linkPicker.ViewOverlay(120, 24, ""))
	t.Logf("\n%s", out)
	for _, url := range []string{"https://a.example/docs", "https://c.example/x"} {
		if n := strings.Count(out, url); n != 1 {
			t.Errorf("%s is printed %d times, want once", url, n)
		}
	}
}

// The filter reads the label, the channel and a drawn URL, never the
// date of the fallback text or a preview: "20" keeps the rows named with
// 20, not every row whose preview has not replaced "May 20, 2026" yet.
func TestLinkPicker_BigTable_FilterIgnoresDateAndPreview(t *testing.T) {
	app, cmd := bigTableApp(t)
	pickerKeys(app, typed("/20")...)
	want := []string{"job 17 (Q20), Sam and D… · 1", "job 17 (Q20), Sam and D… · 2", "job 20 (Q23), Sam and D… · 1"}
	labels := func() (got []string) {
		for _, it := range app.linkPicker.ItemsInView() {
			got = append(got, strings.TrimSpace(it.Label))
		}
		return got
	}
	if got := labels(); !slices.Equal(got, want) {
		t.Errorf("filter \"20\" shows %q, want %q", got, want)
	}
	pickerKeys(app, tea.KeyPressMsg{Code: tea.KeyDown})
	for _, m := range drainCmd(cmd) {
		app.Update(m)
	}
	if got := labels(); !slices.Equal(got, want) || app.linkPicker.Selected() != 26 {
		t.Errorf("after previews landed: %q with row %d highlighted, want the same rows and row 26", got, app.linkPicker.Selected())
	}
}

// A taller terminal shows more rows; their previews are fetched with
// the resize, not at the next key.
func TestLinkPicker_BigTable_ResizeFetchesRowsNowInView(t *testing.T) {
	app, cmd := bigTableApp(t)
	if n := len(drainCmd(cmd)); n != 14 {
		t.Fatalf("opening fetched %d previews, want 14", n)
	}
	_, resized := app.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	if n := len(drainCmd(resized)); n != 6 {
		t.Errorf("growing the terminal to 40 lines fetched %d previews, want the 6 rows that came into view", n)
	}
}

// A context that is blank as plain text is no context: the label does
// not start with the separator, and it alone does not make a column.
func TestLinkPickerLabels_BlankContext(t *testing.T) {
	got, whole := linkPickerLabels([]messages.Link{
		{URL: "https://a.example", Label: "1", Context: "   "},
		{URL: "https://b.example", Label: "docs"},
	}, strings.TrimSpace)
	if !slices.Equal(got, []string{"1", "docs"}) || whole[0] != "1" {
		t.Errorf("labels = %q, whole = %q; want the link text alone", got, whole)
	}
}

// A label holds its 28 cells as lipgloss counts them, with emoji the
// cut used to count narrower (❤️, 1️⃣) in the row's name and the link text.
func TestLinkPickerLabels_EmojiCells(t *testing.T) {
	got, _ := linkPickerLabels([]messages.Link{
		{URL: "https://a.example", Label: "1️⃣ 2️⃣ 3️⃣ release notes ❤️", Context: "❤️ 1️⃣ 2️⃣ 3️⃣ 4️⃣ 5️⃣ 6️⃣ 7️⃣ 8️⃣ 9️⃣ job"},
		{URL: "https://b.example", Label: strings.Repeat("❤️", 30)},
	}, strings.TrimSpace)
	for i, label := range got {
		if w := lipgloss.Width(label); w != linkLabelWidth {
			t.Errorf("label %d is %d cells, want %d: %q", i, w, linkLabelWidth, label)
		}
	}
}
