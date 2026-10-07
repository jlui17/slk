package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

// colonyReviewURL is the !review-open post Colony's "Complete review"
// button links: a top-level message of the judge channel, no replies.
const colonyReviewURL = "https://colony-pyo1658.slack.com/archives/" + judgeChannelID + "/p1790979429246289"

// colonyEphemeral is the ephemeral Colony posts after an annotation: its
// section links the annotated reply, its button the review to complete.
func colonyEphemeral() messages.MessageItem {
	return messages.MessageItem{
		TS:          "1790979500.000100",
		UserName:    "Colony",
		Text:        "Your annotation was sent.",
		Blocks:      blockkittest.ColonyEphemeral("https://colony-pyo1658.slack.com/archives/C0BCG30UGEP/p1790979381013049?thread_ts=1790910827.409349&cid=C0BCG30UGEP", colonyReviewURL),
		IsEphemeral: true,
	}
}

// pickCompleteReview presses key on the selected Colony ephemeral and
// takes the picker's second row, the button, to the OpenLinkMsg it gives.
func pickCompleteReview(t *testing.T, a *App, key rune) OpenLinkMsg {
	t.Helper()
	a.handleNormalMode(tea.KeyPressMsg{Code: key, Text: string(key)})
	if a.mode != ModeLinkPicker {
		t.Fatalf("mode = %v, want the link picker", a.mode)
	}
	items := a.linkPicker.Items()
	if len(items) != 2 || items[0].Label != "this message" || items[1].Label != "Complete review" {
		t.Fatalf("picker rows = %+v, want \"this message\" then \"Complete review\"", items)
	}
	a.handleKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	open, ok := a.handleKey(tea.KeyPressMsg{Code: tea.KeyEnter})().(OpenLinkMsg)
	if !ok {
		t.Fatal("Enter in the picker gave no OpenLinkMsg")
	}
	if open.URL != colonyReviewURL {
		t.Fatalf("opened %q, want the button's %q", open.URL, colonyReviewURL)
	}
	return open
}

// In herdr, O on the button's row opens the linked post in a new tab.
func TestShiftO_LinkButton_OpensInHerdrTab(t *testing.T) {
	a, _ := reviewLinkApp(t)
	var tabURL string
	a.SetHerdrTabOpener(func(url, label string, focus bool) error {
		tabURL = url
		return nil
	})
	a.focusedPanel = PanelMessages
	a.messagepane.SetMessages([]messages.MessageItem{colonyEphemeral()})

	open := pickCompleteReview(t, a, 'O')
	if !open.InHerdrTab {
		t.Errorf("OpenLinkMsg = %+v, want InHerdrTab", open)
	}
	_, cmd := a.Update(open)
	drainCmd(cmd)
	if tabURL != colonyReviewURL {
		t.Errorf("herdr tab opened %q, want %q", tabURL, colonyReviewURL)
	}
}

// o on the button's row goes to the linked post in place, as o on any
// permalink does.
func TestO_LinkButton_SelectsInChannel(t *testing.T) {
	a, _ := reviewLinkApp(t)
	a.focusedPanel = PanelMessages
	a.messagepane.SetMessages([]messages.MessageItem{colonyEphemeral()})

	_, cmd := a.Update(pickCompleteReview(t, a, 'o'))
	sel, ok := findChannelSelected(cmd())
	if !ok || sel.ID != judgeChannelID {
		t.Fatalf("ChannelSelectedMsg = %+v ok=%v, want %s", sel, ok, judgeChannelID)
	}
	_, cmd = a.Update(sel)
	drainCmd(cmd)
	_, cmd = a.Update(MessagesLoadedMsg{ChannelID: judgeChannelID, Messages: judgeHistory()})
	drainCmd(cmd)
	if got, ok := a.messagepane.SelectedMessage(); !ok || got.TS != judgePostTS {
		t.Errorf("channel cursor = %+v ok=%v, want the linked post %s", got, ok, judgePostTS)
	}
}
