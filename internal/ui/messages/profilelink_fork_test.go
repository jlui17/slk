package messages

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A user mention links to the user's profile in the workspace, and is
// plain without one; @here and #channel mentions are never links.
func TestUserMentionLinksToProfile(t *testing.T) {
	text := "hi <@U2> and <!here> in <#C9|general>"
	names := map[string]string{"U2": "dana"}
	drawn := RenderSlackMarkdownWith(text, RenderSlackMarkdownOpts{UserNames: names, WorkspaceDomain: "acme"})
	plain := ansi.Strip(drawn)
	links := map[string]string{}
	for _, word := range []string{"@dana", "@here", "#general"} {
		links[word] = LinkAt(drawn, strings.Index(plain, word)+1)
	}
	want := map[string]string{"@dana": "https://acme.slack.com/team/U2", "@here": "", "#general": ""}
	for word, url := range want {
		if links[word] != url {
			t.Errorf("link at %s = %q, want %q", word, links[word], url)
		}
	}

	drawn = RenderSlackMarkdownWith(text, RenderSlackMarkdownOpts{UserNames: names})
	if got := LinkAt(drawn, strings.Index(ansi.Strip(drawn), "@dana")+1); got != "" {
		t.Errorf("without a domain @dana links to %q", got)
	}
}

// The o picker lists a message's links from its text, so a mention
// never shows up there.
func TestMessageLinksLeaveMentionsOut(t *testing.T) {
	links := MessageLinks(MessageItem{Text: "ask <@U2> about <https://example.com|this>"})
	if len(links) != 1 || links[0].URL != "https://example.com" {
		t.Errorf("MessageLinks = %+v, want only https://example.com", links)
	}
}
