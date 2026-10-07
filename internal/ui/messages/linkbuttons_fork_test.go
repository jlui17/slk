package messages

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

// colonyEphemeral is the message the Colony app posts with
// chat.postEphemeral after an annotation.
func colonyEphemeral() MessageItem {
	return MessageItem{
		TS:          "1787400000.000200",
		UserName:    "Colony",
		Timestamp:   "3:04 PM",
		Text:        "Your annotation was sent.",
		Blocks:      blockkittest.ColonyEphemeral("https://x.slack.com/archives/C1/p1", "https://x.slack.com/archives/C1/p1787300000000100"),
		IsEphemeral: true,
	}
}

// o offers a link button as a row named by its text, after the links of
// the section above it, and c offers the same URLs.
func TestMessageLinks_LinkButton(t *testing.T) {
	msg := colonyEphemeral()
	want := []Link{
		{URL: "https://x.slack.com/archives/C1/p1", Label: "this message"},
		{URL: "https://x.slack.com/archives/C1/p1787300000000100", Label: "Complete review"},
	}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
	var copied []Link
	for _, c := range Copyables(msg) {
		copied = append(copied, c.Link)
	}
	if !reflect.DeepEqual(copied, want) {
		t.Errorf("Copyables links:\n got %#v\nwant %#v", copied, want)
	}
}

// A section's link-button accessory is a row too; a button without a URL
// is not.
func TestMessageLinks_AccessoryLinkButton(t *testing.T) {
	msg := MessageItem{Blocks: []blockkit.Block{
		blockkit.SectionBlock{Text: "Build failed", Accessory: blockkit.LabelAccessory{Kind: "button", Label: "View logs", ButtonFields: blockkit.ButtonFields{URL: "https://ci.example/42"}}},
		blockkit.ActionsBlock{Elements: []blockkit.ActionElement{{Kind: "button", Label: "Retry", ButtonFields: blockkit.ButtonFields{ActionID: "retry"}}}},
	}}
	want := []Link{{URL: "https://ci.example/42", Label: "View logs"}}
	if got := MessageLinks(msg); !reflect.DeepEqual(got, want) {
		t.Errorf("MessageLinks:\n got %#v\nwant %#v", got, want)
	}
}

func TestInteractHint(t *testing.T) {
	link := blockkit.ButtonFields{URL: "https://x.example"}
	actions := func(els ...blockkit.ActionElement) []blockkit.Block {
		return []blockkit.Block{blockkit.ActionsBlock{Elements: els}}
	}
	cases := []struct {
		name string
		msg  MessageItem
		want string
	}{
		{"every control a link button", colonyEphemeral(), "o to open"},
		{"a button without a URL", MessageItem{Blocks: actions(
			blockkit.ActionElement{Kind: "button", Label: "Open", ButtonFields: link},
			blockkit.ActionElement{Kind: "button", Label: "Approve", ButtonFields: blockkit.ButtonFields{ActionID: "approve"}},
		)}, "↗ open in Slack to interact"},
		{"a select beside a link button", MessageItem{Blocks: actions(
			blockkit.ActionElement{Kind: "button", Label: "Open", ButtonFields: link},
			blockkit.ActionElement{Kind: "static_select", Label: "Pick"},
		)}, "↗ open in Slack to interact"},
		{"a non-link accessory", MessageItem{Blocks: []blockkit.Block{
			blockkit.SectionBlock{Text: "x", Accessory: blockkit.LabelAccessory{Kind: "overflow"}},
		}}, "↗ open in Slack to interact"},
		{"a link button in a card, which o does not read", MessageItem{LegacyAttachments: []blockkit.LegacyAttachment{
			{Blocks: actions(blockkit.ActionElement{Kind: "button", Label: "Open", ButtonFields: link})},
		}}, "↗ open in Slack to interact"},
	}
	for _, c := range cases {
		if got := InteractHint(c.msg); got != c.want {
			t.Errorf("%s: InteractHint = %q, want %q", c.name, got, c.want)
		}
	}
}

// The pane draws Colony's ephemeral with "o to open" under its button and
// "only visible to you" on its header row; a message Slack showed to
// everyone has no such mark.
func TestRenderColonyEphemeral(t *testing.T) {
	msg := colonyEphemeral()
	plain := ansi.Strip(renderedFor(t, msg, 100))
	lines := strings.Split(plain, "\n")
	if !strings.Contains(lines[0], "Colony") || !strings.Contains(lines[0], "only visible to you") {
		t.Errorf("header row = %q, want the author and \"only visible to you\"", lines[0])
	}
	if !strings.Contains(plain, "o to open") || strings.Contains(plain, "open in Slack to interact") {
		t.Errorf("want the \"o to open\" hint alone, got %q", plain)
	}

	msg.IsEphemeral = false
	if plain := ansi.Strip(renderedFor(t, msg, 100)); strings.Contains(plain, "only visible to you") {
		t.Errorf("a message everyone sees is marked: %q", plain)
	}
}
