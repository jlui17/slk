package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core/blocks"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/messages"
)

const ephTS = "5.000000"

// mentionEphemeral is Slackbot's "not in channel" notice as Part A parses it.
func mentionEphemeral(ts string) messages.MessageItem {
	return messages.MessageItem{
		TS: ts, UserID: "USLACKBOT", UserName: "Slackbot", Ephemeral: true, BotID: "B01",
		Text: "You mentioned <@U0EXAMPLE01>, but they're not in this channel.",
		LegacyAttachments: []blocks.LegacyAttachment{{
			ID: 1, CallbackID: "cb", Text: "You may want to invite them.",
			Actions: []blocks.LegacyAction{
				{ID: "1", Name: "invite", Text: "Add Them", Type: "button", Value: "invite",
					Confirm: &blocks.ActionConfirm{Title: "Are you sure you want to add them?", Text: "History is visible.", OKText: "Add", DismissText: "Cancel"}},
				{ID: "2", Name: "ignore", Text: "Dismiss", Type: "button", Value: "ignore"},
				{ID: "3", Name: "dont-show-again", Text: "Don't Show Again", Type: "button", Value: "dont-show-again"},
			},
		}},
	}
}

type pressCall struct {
	channel, ts string
	attID       int
	callbackID  string
	serviceID   string
	action      blocks.LegacyAction
	ephemeral   bool
}

// actionApp is an App on C1 showing msgs, whose presses are recorded and
// answered with err.
func actionApp(t *testing.T, err error, msgs ...messages.MessageItem) (*App, *[]pressCall) {
	t.Helper()
	a := newTestApp(t, withSize(120, 40), withActiveChannel("C1"), withMessages(msgs...))
	calls := &[]pressCall{}
	a.setInteractionPressForTest(func(ch ids.ChannelID, ts ids.MessageTS, attID int, cb, svcID string, act blocks.LegacyAction, eph bool) error {
		*calls = append(*calls, pressCall{string(ch), string(ts), attID, cb, svcID, act, eph})
		return err
	})
	return a, calls
}

// pressKey sends one key to the App and runs every resulting command back
// through Update, returning the messages produced.
func pressKey(t *testing.T, a *App, k tea.KeyMsg) []tea.Msg {
	t.Helper()
	_, cmd := a.Update(k)
	var out []tea.Msg
	for _, m := range drainBatch(cmd) {
		if m == nil {
			continue
		}
		out = append(out, m)
		_, next := a.Update(m)
		out = append(out, drainBatch(next)...)
	}
	return out
}

func toastText(msgs []tea.Msg) string {
	for _, m := range msgs {
		if t, ok := m.(ToastMsg); ok {
			return t.Text
		}
	}
	return ""
}

func TestOpenActionPromptShowsButtons(t *testing.T) {
	a, _ := actionApp(t, nil, mentionEphemeral(ephTS))
	if !a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS}) {
		t.Fatal("openActionPrompt = false")
	}
	if a.mode != ModeConfirm {
		t.Errorf("mode = %v, want ModeConfirm", a.mode)
	}
	view := a.confirmPrompt.View()
	for _, want := range []string{"Slackbot", "not in this channel", "invite them", "[1] Add Them", "[2] Dismiss", "[3] Don't Show Again"} {
		if !strings.Contains(view, want) {
			t.Errorf("dialog missing %q:\n%s", want, view)
		}
	}
	if a.confirmPrompt.Highlighted() != -1 {
		t.Error("no button may start highlighted")
	}
}

func TestOpenActionPromptMissingMessage(t *testing.T) {
	a, _ := actionApp(t, nil)
	if a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS}) {
		t.Error("openActionPrompt should fail when the message is not in a pane")
	}
}

func TestActionPromptPressSendsTheAction(t *testing.T) {
	a, calls := actionApp(t, nil, mentionEphemeral(ephTS))
	a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS})

	if toast := toastText(pressKey(t, a, keyPress('2'))); toast != "" {
		t.Errorf("a successful press toasted %q", toast)
	}
	if len(*calls) != 1 {
		t.Fatalf("presses = %d, want 1", len(*calls))
	}
	c := (*calls)[0]
	if c.channel != "C1" || c.ts != ephTS || c.attID != 1 || c.callbackID != "cb" || c.serviceID != "B01" ||
		c.action.ID != "2" || c.action.Name != "ignore" || !c.ephemeral {
		t.Errorf("press = %+v", c)
	}
	if a.confirmPrompt.IsVisible() || a.mode != ModeNormal {
		t.Error("the dialog should close on press")
	}
}

func TestActionPromptFailedPressToasts(t *testing.T) {
	a, _ := actionApp(t, errors.New("boom"), mentionEphemeral(ephTS))
	a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS})

	if got, want := toastText(pressKey(t, a, keyPress('2'))), "Couldn't send Dismiss: boom"; got != want {
		t.Errorf("toast = %q, want %q", got, want)
	}
	if _, ok := a.findActionMessage(actionPromptRef{channelID: "C1", ts: ephTS}); !ok {
		t.Error("the message must stay so the press can be retried")
	}
}

func TestActionPromptConfirmStep(t *testing.T) {
	a, calls := actionApp(t, nil, mentionEphemeral(ephTS))
	a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS})

	pressKey(t, a, keyPress('1'))
	if !strings.Contains(a.confirmPrompt.View(), "Are you sure you want to add them?") || len(*calls) != 0 {
		t.Fatalf("Add Them should ask first, without pressing:\n%s", a.confirmPrompt.View())
	}
	pressKey(t, a, keyPress('1')) // Cancel: back to the buttons
	if !strings.Contains(a.confirmPrompt.View(), "[2] Dismiss") {
		t.Fatalf("Cancel should return to the buttons:\n%s", a.confirmPrompt.View())
	}
	pressKey(t, a, keyPress('1'))
	pressKey(t, a, keyPress('2')) // Add
	if len(*calls) != 1 || (*calls)[0].action.Name != "invite" {
		t.Errorf("presses = %+v, want one invite", *calls)
	} else if (*calls)[0].serviceID != "B01" {
		t.Errorf("serviceID after confirm = %q, want B01", (*calls)[0].serviceID)
	}
}

func TestActionPromptURLButtonOpensLink(t *testing.T) {
	msg := mentionEphemeral(ephTS)
	msg.LegacyAttachments[0].Actions[1].URL = "https://example.com/docs"
	a, calls := actionApp(t, nil, msg)
	a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS})

	var opened string
	_, cmd := a.Update(keyPress('2'))
	for _, m := range drainBatch(cmd) {
		if o, ok := m.(OpenLinkMsg); ok {
			opened = o.URL
		}
	}
	if opened != "https://example.com/docs" || len(*calls) != 0 {
		t.Errorf("opened %q, presses %d; want the URL and no press", opened, len(*calls))
	}
}

// Review Focus 5: buttons past the ninth have no number key but are still
// reachable with Tab.
func TestActionPromptTenthButtonReachableByTab(t *testing.T) {
	msg := mentionEphemeral(ephTS)
	var acts []blocks.LegacyAction
	for i := 1; i <= 10; i++ {
		acts = append(acts, blocks.LegacyAction{ID: fmt.Sprint(i), Name: fmt.Sprint("b", i), Text: fmt.Sprint("B", i), Type: "button"})
	}
	msg.LegacyAttachments[0].Actions = acts
	a, calls := actionApp(t, nil, msg)
	a.openActionPrompt(actionPromptRef{channelID: "C1", ts: ephTS})

	if strings.Contains(a.confirmPrompt.View(), "[10]") {
		t.Error("the tenth button must not get a number key")
	}
	for range 10 {
		pressKey(t, a, keyCode(tea.KeyTab))
	}
	pressKey(t, a, keyCode(tea.KeyEnter))
	if len(*calls) != 1 || (*calls)[0].action.Name != "b10" {
		t.Errorf("presses = %+v, want b10", *calls)
	}
}

func TestButtonActionsSkipsSelects(t *testing.T) {
	msg := mentionEphemeral(ephTS)
	msg.LegacyAttachments[0].Actions = []blocks.LegacyAction{{Name: "env", Text: "Env", Type: "select"}}
	if messages.HasButtonActions(msg) || len(messages.ButtonActions(msg)) != 0 {
		t.Error("select actions are not buttons")
	}
}
