package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The text spells the permalink the way Slack's mrkdwn does (& as
// &amp;); the card's from_url spells it plain, with the query in
// another order.
func chipMessage() MessageItem {
	msg := cardMessage("1736093400.000100", linkedMessageCard("?cid=C0000000001&thread_ts=1736000000.000100", "short"))
	msg.Text = "note (<" + cardPermalink("?thread_ts=1736000000.000100&amp;cid=C0000000001") + ">) and <https://example.slack.com/archives/C0000000002/p1736000000000300> too"
	return msg
}

func TestPermalinkChip(t *testing.T) {
	msg := chipMessage()
	carded := cardPermalink("?thread_ts=1736000000.000100&amp;cid=C0000000001")
	out := RenderSlackMarkdownWith(msg.Text, RenderSlackMarkdownOpts{PermalinkChips: PermalinkChipsOf(msg, cardChannelNames)})

	plain := ansi.Strip(out)
	if want := "note (↳ Claude in #dev) and https://example.slack.com/archives/C0000000002/p1736000000000300 too"; plain != want {
		t.Errorf("body = %q, want %q", plain, want)
	}
	// A hyperlink to the full URL (entities decoded, as for any link),
	// in the link style.
	linkSGR, _, _ := strings.Cut(linkStyle().Render("x"), "x")
	if want := "\x1b]8;;" + strings.ReplaceAll(carded, "&amp;", "&") + "\x1b\\" + linkSGR + "↳"; !strings.Contains(out, want) {
		t.Errorf("chip must be a styled hyperlink to the full URL; want %q in %q", want, out)
	}
}

func TestPermalinkChip_AuthorsLabelStays(t *testing.T) {
	msg := chipMessage()
	msg.Text = "<" + cardPermalink("") + "|the thread>"
	out := RenderSlackMarkdownWith(msg.Text, RenderSlackMarkdownOpts{PermalinkChips: PermalinkChipsOf(msg, cardChannelNames)})
	if plain := ansi.Strip(out); plain != "the thread" {
		t.Errorf("body = %q, want the author's label", plain)
	}
}

func TestPermalinkChip_WithoutAChannelName(t *testing.T) {
	msg := chipMessage()
	out := RenderSlackMarkdownWith("<"+cardPermalink("")+">", RenderSlackMarkdownOpts{PermalinkChips: PermalinkChipsOf(msg, nil)})
	if plain := ansi.Strip(out); plain != "↳ Claude" {
		t.Errorf("body = %q, want %q", plain, "↳ Claude")
	}
}

// o/O, the link picker and c read the message, not the rendering: they
// still see the full URL.
func TestPermalinkChip_LinksAndCopyKeepTheFullURL(t *testing.T) {
	msg := chipMessage()
	carded := cardPermalink("?thread_ts=1736000000.000100&amp;cid=C0000000001")
	if links := ExtractLinks(msg.Text); len(links) != 2 || links[0].URL != carded {
		t.Errorf("ExtractLinks = %+v, want the full permalink first", links)
	}
	if items := Copyables(msg); len(items) != 2 || items[0].Text != strings.ReplaceAll(carded, "&amp;", "&") {
		t.Errorf("Copyables = %+v, want the full permalink first", items)
	}
}

func TestPermalinkChip_InTheMessagesPane(t *testing.T) {
	m := newCardModel(chipMessage())
	plain := viewPlain(m, 30, 100)
	if !strings.Contains(plain, "note (↳ Claude in #dev) and https://example.slack.com/archives/C0000000002/") {
		t.Errorf("pane body lacks the chip or lost the uncarded permalink:\n%s", plain)
	}
}
