package ui

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/usernames"
)

func dateLabel(ts string) string {
	return messages.FormatDateSeparator(messages.DateFromTS(ts))
}

// Permalink rows open with a decoded fallback display: channel + date
// for in-app links, subdomain + date for foreign workspaces, "thread
// reply" marker when the permalink carries a thread_ts. Non-permalink
// rows keep showing their URL (empty Display).
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
	if want := "#general · " + dateLabel("1779284733.270139"); items[0].Display != want {
		t.Errorf("in-app Display = %q, want %q", items[0].Display, want)
	}
	if want := "#general · " + dateLabel("1779284734.000000") + " · thread reply"; items[1].Display != want {
		t.Errorf("thread-reply Display = %q, want %q", items[1].Display, want)
	}
	if want := "otherteam.slack.com · " + dateLabel("1779284733.270139"); items[2].Display != want {
		t.Errorf("foreign Display = %q, want %q", items[2].Display, want)
	}
	if items[3].Display != "" {
		t.Errorf("non-permalink Display = %q, want empty", items[3].Display)
	}
}

func linkPreviewTestApp(t *testing.T) *App {
	t.Helper()
	app, _ := linkTestApp(t)
	app.SetUserNames(usernames.FromMap(map[string]string{"U1": "matt"}))
	app.channelNames = map[string]string{"C054JFCBN69": "general"}
	app.SetMessageService(NewMessageService(MessageServiceFuncs{
		Preview: func(ctx context.Context, channelID ids.ChannelID, ts ids.MessageTS, threadTS ids.ThreadTS) (string, string, error) {
			return "U1", "deploy is done\nsee <#C054JFCBN69> for details", nil
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
// each result fills its row with "#channel · sender: flattened text".
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
	want := "#general · matt: deploy is done see #general for details"
	if items[0].Display != want {
		t.Errorf("Display = %q, want %q", items[0].Display, want)
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
	if got := app.linkPicker.Items()[0].Display; got != "#general · "+dateLabel("1779284733.270139") {
		t.Errorf("Display = %q, want untouched fallback", got)
	}
}

// Raw text that flattens to nothing must keep the fallback row, not
// overwrite it with a dangling "sender: ".
func TestLinkPicker_EmptyFlattenKeepsFallback(t *testing.T) {
	app := linkPreviewTestApp(t)
	msgs := drainCmd(pressO(app))
	pm := msgs[0].(LinkPreviewMsg)
	pm.Text = "   \n\t "
	app.Update(pm)
	if got := app.linkPicker.Items()[0].Display; got != "#general · "+dateLabel("1779284733.270139") {
		t.Errorf("Display = %q, want untouched fallback", got)
	}
}
