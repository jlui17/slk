package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/help"
	"github.com/gammons/slk/internal/ui/messages"
)

const guardText = "Only visible to you — press b to respond"

// selectedEphemeralApp has a normal message and then msg in C1, msg
// selected in a focused messages pane.
func selectedEphemeralApp(t *testing.T, msg messages.MessageItem) (*App, *[]pressCall) {
	t.Helper()
	a, calls := actionApp(t, nil, messages.MessageItem{TS: "1.000000", UserID: "U1", UserName: "alice", Text: "hi"}, msg)
	a.focusedPanel = PanelMessages
	a.messagepane.SelectByIndex(1)
	return a, calls
}

func TestBOpensDialogForSelectedEphemeral(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	pressKey(t, a, keyPress('b'))
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("b on an ephemeral with buttons should open its dialog")
	}
}

func TestBWithoutButtons(t *testing.T) {
	plain := mentionEphemeral(ephTS)
	plain.LegacyAttachments = nil
	for name, a := range map[string]*App{
		"ephemeral without buttons": func() *App { a, _ := selectedEphemeralApp(t, plain); return a }(),
		"normal message": func() *App {
			a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
			a.messagepane.SelectByIndex(0)
			return a
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			if got := toastText(pressKey(t, a, keyPress('b'))); got != "No buttons on this message" {
				t.Errorf("toast = %q", got)
			}
			if a.confirmPrompt.IsVisible() {
				t.Error("no dialog without buttons")
			}
		})
	}
}

// The keys that would send an ephemeral's ts to Slack are inert on it.
func TestMessageOpKeysAreInertOnEphemeral(t *testing.T) {
	for _, k := range []tea.KeyMsg{
		keyCode(tea.KeyEnter), keyPress('r'), keyPress('R'), keyPress('L'),
		keyPress('Y'), keyPress('C'), keyPress('F'), keyPress('U'), keyPress('S'),
	} {
		t.Run(k.String(), func(t *testing.T) {
			a, calls := selectedEphemeralApp(t, mentionEphemeral(ephTS))
			if got := toastText(pressKey(t, a, k)); got != guardText {
				t.Errorf("toast = %q, want %q", got, guardText)
			}
			if a.mode != ModeNormal || a.threadVisible || len(*calls) != 0 {
				t.Errorf("mode=%v threadVisible=%v presses=%d: the key must do nothing else", a.mode, a.threadVisible, len(*calls))
			}
		})
	}
}

func TestGuardTextWithoutButtons(t *testing.T) {
	plain := mentionEphemeral(ephTS)
	plain.LegacyAttachments = nil
	a, _ := selectedEphemeralApp(t, plain)
	if got := toastText(pressKey(t, a, keyPress('r'))); got != "Only visible to you" {
		t.Errorf("toast = %q", got)
	}
}

func TestCopyStillWorksOnEphemeral(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	if got := toastText(pressKey(t, a, keyPress('y'))); strings.HasPrefix(got, "Only visible to you") {
		t.Errorf("y must not be guarded; toast = %q", got)
	}
}

func TestGuardLeavesNormalMessagesAlone(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	a.messagepane.SelectByIndex(0)
	if got := toastText(pressKey(t, a, keyPress('r'))); strings.HasPrefix(got, "Only visible to you") {
		t.Errorf("a normal message must not be guarded; toast = %q", got)
	}
}

func TestBDoesNotReopenAfterClose(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	a.actionQueue = []actionPromptRef{{channelID: "C1", ts: ephTS}} // queued earlier, while typing
	pressKey(t, a, keyPress('b'))
	pressKey(t, a, keyCode(tea.KeyEscape))
	if a.confirmPrompt.IsVisible() {
		t.Error("opening with b must take the message off the queue")
	}
}

func TestHelpListsB(t *testing.T) {
	for _, e := range help.FromKeyMap(DefaultKeyMap()) {
		if e.Key == "b" && e.Desc == "respond to message buttons" {
			return
		}
	}
	t.Error("help overlay is missing b")
}

// In the activity and threads views the messages-pane slot shows a list, not
// the channel's messages: the stale channel selection must not be guarded.
func TestGuardIgnoresHiddenChannelPane(t *testing.T) {
	for name, view := range map[string]View{"activity": ViewActivity, "threads": ViewThreads} {
		t.Run(name, func(t *testing.T) {
			a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
			a.view = view
			if got := toastText(pressKey(t, a, keyCode(tea.KeyEnter))); strings.HasPrefix(got, "Only visible to you") {
				t.Errorf("Enter must not be guarded; toast = %q", got)
			}
			if got := toastText(pressKey(t, a, keyPress('b'))); got != "No buttons on this message" {
				t.Errorf("b toast = %q", got)
			}
			if a.confirmPrompt.IsVisible() {
				t.Error("b must not open a dialog for a message that is not on screen")
			}
		})
	}
}

func TestGuardIgnoresEphemeralWhenSidebarFocused(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	a.focusedPanel = PanelSidebar
	if got := toastText(pressKey(t, a, keyCode(tea.KeyEnter))); strings.HasPrefix(got, "Only visible to you") {
		t.Errorf("Enter must not be guarded; toast = %q", got)
	}
}

func TestEphemeralReplyInThreadPanel(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	openThreadPanel(a, "C1", "1.000000")
	reply := mentionEphemeral(ephTS)
	reply.ThreadTS = "1.000000"
	a.threadPanel.AddReply(reply)
	a.focusedPanel = PanelThread
	a.threadPanel.SelectByIndex(0)

	if got := toastText(pressKey(t, a, keyPress('r'))); got != guardText {
		t.Errorf("r toast = %q, want %q", got, guardText)
	}
	pressKey(t, a, keyPress('b'))
	if !dialogShows(a, "[2] Dismiss") {
		t.Error("b on an ephemeral reply should open its dialog")
	}
}

// A click on a row opens its thread through openThreadForSelectedMessage,
// not through the key guard: the ephemeral's ts is a ghost to Slack.
func TestClickOnEphemeralDoesNotOpenThread(t *testing.T) {
	a, _ := selectedEphemeralApp(t, mentionEphemeral(ephTS))
	a.focusedPanel = PanelSidebar // a click does not need the pane focused first
	fetched := 0
	a.setThreadFetcherForTest(func(ids.ChannelID, ids.ThreadTS) core.Msg {
		fetched++
		return nil
	})
	cmd := a.openThreadForSelectedMessage()
	if got := toastText(drainBatch(cmd)); got != guardText {
		t.Errorf("toast = %q, want %q", got, guardText)
	}
	if a.threadVisible || fetched != 0 {
		t.Errorf("threadVisible=%v fetches=%d: the click must not open a thread", a.threadVisible, fetched)
	}
}
