package ui

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/usernames"
)

func dateLabel(ts string) string {
	return messages.FormatDateSeparator(messages.DateFromTS(ts))
}

// Permalink rows open with a decoded fallback: the date as the text,
// with a "thread reply" marker when the permalink carries a thread_ts,
// and the channel of an in-app link or the subdomain of a foreign
// workspace in the Side column. Non-permalink rows keep showing their
// URL (empty Display, no Side).
func TestOpenLinkKey_PermalinkRows_DecodedDisplay(t *testing.T) {
	app, _ := linkTestApp(t)
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139> " +
			"<https://myteam.slack.com/archives/C054JFCBN69/p1779284734000000?thread_ts=1779284733.270139&amp;cid=C054JFCBN69> " +
			"<https://otherteam.slack.com/archives/C054JFCBN69/p1779284733270139> " +
			"<https://example.com/x>"},
	})
	pressO(app)
	items := app.linkPicker.Items()
	if len(items) != 4 {
		t.Fatalf("items = %#v, want 4", items)
	}
	if want := dateLabel("1779284733.270139"); items[0].Display != want || items[0].Side != "#general" {
		t.Errorf("in-app Display = %q, Side = %q; want %q and #general", items[0].Display, items[0].Side, want)
	}
	if want := dateLabel("1779284734.000000") + " · thread reply"; items[1].Display != want || items[1].Side != "#general" {
		t.Errorf("thread-reply Display = %q, Side = %q; want %q and #general", items[1].Display, items[1].Side, want)
	}
	if want := dateLabel("1779284733.270139"); items[2].Display != want || items[2].Side != "otherteam.slack.com" {
		t.Errorf("foreign Display = %q, Side = %q; want %q and otherteam.slack.com", items[2].Display, items[2].Side, want)
	}
	if items[3].Display != "" || items[3].Side != "" {
		t.Errorf("non-permalink Display = %q, Side = %q; want both empty", items[3].Display, items[3].Side)
	}
	for _, it := range items {
		if it.Detail != "" {
			t.Errorf("Detail = %q, want none: a permalink row no longer draws its URL", it.Detail)
		}
	}
}

func linkPreviewTestApp(t *testing.T) *App {
	t.Helper()
	app, _ := linkTestApp(t)
	app.SetUserNames(usernames.FromMap(map[string]string{"U1": "matt"}))
	app.channelNames = map[string]string{"C054JFCBN69": "general"}
	app.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{
		Preview: func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			return "matt", "deploy is done\nsee <#C054JFCBN69> for details", nil
		},
	}))
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{
		{TS: "1.0", Text: "<https://myteam.slack.com/archives/C054JFCBN69/p1779284733270139> " +
			"<https://example.com/x>"},
	})
	return app
}

// Opening the picker fetches previews for in-app permalink rows only;
// each result fills its row with the flattened text, and its Side with
// "#channel · sender".
func TestLinkPicker_PreviewFillsRow(t *testing.T) {
	app := linkPreviewTestApp(t)
	cmd := pressO(app)
	msgs := drainCmd(cmd)
	if len(msgs) != 1 {
		t.Fatalf("preview msgs = %#v, want 1 (in-app row only)", msgs)
	}
	pm, ok := msgs[0].(LinkPreviewMsg)
	if !ok || pm.Index != 0 {
		t.Fatalf("got %#v, want LinkPreviewMsg for row 0", msgs[0])
	}
	app.Update(pm)
	items := app.linkPicker.Items()
	want := "deploy is done see #general for details"
	if items[0].Display != want || items[0].Side != "#general · matt" {
		t.Errorf("Display = %q, Side = %q; want %q and \"#general · matt\"", items[0].Display, items[0].Side, want)
	}
	if items[1].Display != "" {
		t.Errorf("non-permalink Display = %q, want empty", items[1].Display)
	}
}

// A preview from a superseded picker generation must not touch the
// current picker's rows.
func TestLinkPicker_StalePreviewDropped(t *testing.T) {
	app := linkPreviewTestApp(t)
	msgs := drainCmd(pressO(app))
	pm := msgs[0].(LinkPreviewMsg)
	pm.Gen--
	app.Update(pm)
	if got := app.linkPicker.Items()[0]; got.Display != dateLabel("1779284733.270139") || got.Side != "#general" {
		t.Errorf("Display = %q, Side = %q; want untouched fallback", got.Display, got.Side)
	}
}

// Raw text that flattens to nothing must keep the fallback row, not
// overwrite it with an empty one.
func TestLinkPicker_EmptyFlattenKeepsFallback(t *testing.T) {
	app := linkPreviewTestApp(t)
	msgs := drainCmd(pressO(app))
	pm := msgs[0].(LinkPreviewMsg)
	pm.Text = "   \n\t "
	app.Update(pm)
	if got := app.linkPicker.Items()[0]; got.Display != dateLabel("1779284733.270139") || got.Side != "#general" {
		t.Errorf("Display = %q, Side = %q; want untouched fallback", got.Display, got.Side)
	}
}

// A preview whose message names no sender leaves the column as the
// channel alone, not "#general · ".
func TestLinkPicker_PreviewWithoutSender(t *testing.T) {
	app := linkPreviewTestApp(t)
	msgs := drainCmd(pressO(app))
	pm := msgs[0].(LinkPreviewMsg)
	pm.Sender = ""
	app.Update(pm)
	if row := app.linkPicker.Items()[0]; row.Display != "deploy is done see #general for details" || row.Side != "#general" {
		t.Errorf("Display = %q, Side = %q; want the preview and the channel alone", row.Display, row.Side)
	}
}
