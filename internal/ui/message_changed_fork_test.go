package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// Slack sends a link's unfurl after the post, in a message_changed
// event, which reaches the app as a NewMessageMsg with IsEdited set and
// the message's full new content. These are synthetic stand-ins.

const unfurledPermalink = "https://example.slack.com/archives/C0000000001/p1736092920000200?thread_ts=1736092920.000200&amp;cid=C0000000001"

func unfurlCard() []blockkit.LegacyAttachment {
	body := make([]string, 8)
	for i := range body {
		body[i] = fmt.Sprintf("linked line %d", i+1)
	}
	return []blockkit.LegacyAttachment{{
		AuthorName: "Claude",
		FromURL:    strings.ReplaceAll(unfurledPermalink, "&amp;", "&"),
		Footer:     "Thread in Slack Conversation",
		Text:       strings.Join(body, "\n"),
	}}
}

func barePost(ts, threadTS string) messages.MessageItem {
	return messages.MessageItem{TS: ts, ThreadTS: threadTS, UserID: "U1", UserName: "alice", Timestamp: "4:10 PM", Text: "test: <" + unfurledPermalink + ">"}
}

// changed is post as a message_changed event delivers it.
func changed(post messages.MessageItem, text string, card []blockkit.LegacyAttachment) NewMessageMsg {
	post.Text, post.LegacyAttachments, post.IsEdited = text, card, true
	return NewMessageMsg{ChannelID: "C1", Message: post}
}

func unfurlTestApp() *App {
	app := NewApp()
	app.width, app.height = 120, 40
	app.activeChannelID = "C1"
	names := map[string]string{"C0000000001": "dev"}
	app.messagepane.SetChannelNames(names)
	app.threadPanel.SetChannelNames(names)
	app.messagepane.SetMessages([]messages.MessageItem{{TS: "1.0", UserName: "bob", Text: "older"}})
	return app
}

func TestMessageChanged_UnfurlDrawsTheCardAndTheChip_MessagesPane(t *testing.T) {
	app := unfurlTestApp()
	post := barePost("2.0", "")
	app.Update(NewMessageMsg{ChannelID: "C1", Message: post})
	if s := screen(app); !strings.Contains(s, "https://example.slack.com/") || strings.Contains(s, "more lines") {
		t.Fatalf("before the unfurl the post is a bare URL:\n%s", s)
	}

	app.Update(changed(post, post.Text, unfurlCard()))
	s := screen(app)
	if !strings.Contains(s, "test: ↳ Claude in #dev") {
		t.Errorf("the unfurl must turn the permalink into a chip:\n%s", s)
	}
	if !strings.Contains(s, "Claude · Thread in #dev") || !strings.Contains(s, "▸ 5 more lines · z to expand") {
		t.Errorf("the unfurl must draw the card:\n%s", s)
	}
}

func TestMessageChanged_UnfurlDrawsTheCardAndTheChip_ThreadReplyAndParent(t *testing.T) {
	app := unfurlTestApp()
	parent, reply := barePost("3.0", "3.0"), barePost("3.001", "3.0")
	app.threadPanel.SetThread(parent, []messages.MessageItem{reply}, "C1", "3.0")

	app.Update(changed(reply, reply.Text, unfurlCard()))
	if got := ansi.Strip(app.threadPanel.View(60, 80)); strings.Count(got, "test: ↳ Claude in #dev") != 1 || strings.Count(got, "▸ 5 more lines") != 1 {
		t.Errorf("the reply's unfurl must draw its chip and card:\n%s", got)
	}

	app.Update(changed(parent, parent.Text, unfurlCard()))
	if got := ansi.Strip(app.threadPanel.View(60, 80)); strings.Count(got, "test: ↳ Claude in #dev") != 2 || strings.Count(got, "▸ 5 more lines") != 2 {
		t.Errorf("the parent's unfurl must draw its chip and card too:\n%s", got)
	}
}

func TestMessageChanged_TextEditStillUpdatesTextAndMark(t *testing.T) {
	app := unfurlTestApp()
	post := barePost("2.0", "")
	app.Update(NewMessageMsg{ChannelID: "C1", Message: post})
	app.Update(changed(post, post.Text, unfurlCard()))

	app.Update(changed(post, "rewritten: <"+unfurledPermalink+">", unfurlCard()))
	s := screen(app)
	if !strings.Contains(s, "rewritten: ↳ Claude in #dev") || strings.Contains(s, "test: ") {
		t.Errorf("an edit must replace the text:\n%s", s)
	}
	if !strings.Contains(s, "(edited)") || !strings.Contains(s, "▸ 5 more lines") {
		t.Errorf("an edit keeps the edited mark and the card:\n%s", s)
	}

	// An edit that removes the link: Slack sends the message without the
	// attachment, and the card goes with it.
	app.Update(changed(post, "no link now", nil))
	if s := screen(app); !strings.Contains(s, "no link now") || strings.Contains(s, "more lines") {
		t.Errorf("an edit that drops the link drops the card:\n%s", s)
	}
}

func TestMessageChanged_KeepsTheFoldState(t *testing.T) {
	app := unfurlTestApp()
	app.focusedPanel = PanelMessages
	post := barePost("2.0", "")
	app.Update(NewMessageMsg{ChannelID: "C1", Message: post})
	app.Update(changed(post, post.Text, unfurlCard()))
	screen(app)
	pressZ(app)
	if s := screen(app); !strings.Contains(s, "▾ z to collapse") {
		t.Fatalf("z must expand the card:\n%s", s)
	}

	app.Update(changed(post, "rewritten: <"+unfurledPermalink+">", unfurlCard()))
	if s := screen(app); !strings.Contains(s, "rewritten: ") || !strings.Contains(s, "▾ z to collapse") {
		t.Errorf("an update must leave the card expanded:\n%s", s)
	}

	pressZ(app)
	app.Update(changed(post, "rewritten again: <"+unfurledPermalink+">", unfurlCard()))
	if s := screen(app); !strings.Contains(s, "rewritten again: ") || !strings.Contains(s, "▸ 5 more lines") {
		t.Errorf("an update must leave the card collapsed:\n%s", s)
	}
}
