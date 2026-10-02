package ui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

// cellOf is the cell at which sub starts in line, -1 when it is not there.
func cellOf(line, sub string) int {
	before, _, found := strings.Cut(line, sub)
	if !found {
		return -1
	}
	return ansi.StringWidth(before)
}

func screenRows(screen, sub string) (rows []string) {
	for _, l := range strings.Split(screen, "\n") {
		if strings.Contains(l, sub) {
			rows = append(rows, l)
		}
	}
	return rows
}

func landPreviews(app *App, cmd tea.Cmd) {
	for _, m := range drainCmd(cmd) {
		app.Update(m)
	}
}

// A permalink row reads label, preview, then "#channel · sender" in a
// muted column of its own that is where it was before the previews
// landed, however short a preview is.
func TestLinkPicker_PermalinkRow_PreviewThenChannelColumn(t *testing.T) {
	app, cmd := bigTableAppSized(t, 120, 40)
	before := pickerScreen(t, app)
	if w := ansi.StringWidth(screenRows(before, "╭")[0]); w != 119 {
		t.Errorf("box and its margin are %d cells wide, want the widest box, 118 and 1", w)
	}
	rows := screenRows(before, "[slk]")
	if len(rows) != 20 {
		t.Fatalf("%d rows drawn, want 20", len(rows))
	}
	column := cellOf(rows[0], "#general")
	for _, row := range rows {
		if strings.Contains(row, "https://") || cellOf(row, "#general") != column || cellOf(row, "Wednesday, May 20, 2026") >= column {
			t.Fatalf("before the previews: want date, then #general at cell %d, and no URL:\n%s", column, row)
		}
	}

	landPreviews(app, cmd)
	after := pickerScreen(t, app)
	for _, want := range []string{
		" [ ] job 1 (Q4) · 1                the nightly export skipped two regions again     #general · dana          [slk]",
		" [ ] job 1 (Q4) · 2                ok                                               #general · dana          [slk]",
	} {
		if !strings.Contains(after, want) {
			t.Errorf("screen missing row %q", want)
		}
	}
	for _, row := range screenRows(after, "[slk]") {
		if cellOf(row, "#general · dana") != column {
			t.Errorf("the channel column moved from cell %d when the previews landed:\n%s", column, row)
		}
	}
}

// At 80 columns a table's labels leave too little for both: the channel
// column is dropped. A row that is still loading, or whose preview
// failed, says where it goes in its text, as before the column existed;
// a landed preview takes the whole row.
func TestLinkPicker_PermalinkRow_NarrowDropsTheChannelColumn(t *testing.T) {
	app, cmd := bigTableApp(t)
	if want := " [ ] job 1 (Q4) · 1                #general · Wednesday, May 20, 20… [slk]"; !strings.Contains(pickerScreen(t, app), want) {
		t.Errorf("loading: screen missing row %q", want)
	}
	landPreviews(app, cmd)
	out := pickerScreen(t, app)
	if strings.Contains(out, "#general") {
		t.Error("channel drawn in a row whose preview landed, with under 40 cells for the preview and it")
	}
	if want := " [ ] job 1 (Q4) · 1                the nightly export skipped two r… [slk]"; !strings.Contains(out, want) {
		t.Errorf("screen missing row %q", want)
	}
}

// At 80 columns the short labels of a list leave room for both in o's
// picker, which has no checkbox column.
func TestLinkPicker_PermalinkRow_EightyColumns(t *testing.T) {
	app := listLinksApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{listLinksMessage()})
	app.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Preview: func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			return "sam", "please look at the retry loop in the uploader", nil
		},
	}))
	cmd := pressO(app)
	out := pickerScreen(t, app)
	for _, want := range []string{
		"▌1. @dana                 Wednesday, May 20, 2026   #general         [slk]",
		" the summary from Monday  Wednesday, May 20, 2026…  #general         [slk]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing row %q", want)
		}
	}
	landPreviews(app, cmd)
	if want := "▌1. @dana                 please look at the retr…  #general · sam   [slk]"; !strings.Contains(pickerScreen(t, app), want) {
		t.Errorf("screen missing row %q", want)
	}
}

// A wide terminal gives the box 160 columns and the channel column 40
// cells: a 21-cell channel name leaves a long sender readable.
func TestLinkPicker_PermalinkRow_WideTerminal(t *testing.T) {
	app := listLinksApp(t)
	app.width, app.height = 200, 50
	app.setChannelLookupFuncForTest(func(ids.ChannelID) (string, string, bool) {
		return "nightly-export-batch", "channel", true
	})
	app.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Preview: func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			return "Robo [nightly export of the big tables]", "the nightly export skipped two regions again, see the retry loop in the uploader", nil
		},
	}))
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{listLinksMessage()})
	cmd := pressShiftO(app)
	landPreviews(app, cmd)
	out := pickerScreen(t, app)
	if w := ansi.StringWidth(strings.TrimSpace(screenRows(out, "╭")[0])); w != 160 {
		t.Errorf("box is %d cells wide, want 160", w)
	}
	if want := "the nightly export skipped two regions again, see the retry loop in the uploa…  #nightly-export-batch · Robo [nightly e… [slk]"; !strings.Contains(out, want) {
		t.Errorf("screen missing %q", want)
	}
}
