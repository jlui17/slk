package messages

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
)

// Synthetic stand-ins with the shape of a Slack message unfurl: the
// author and permalink Slack sends, the body flat in text and formatted
// in a rich_text block.

var cardLinkedAt = time.Date(2025, 1, 5, 16, 2, 0, 0, time.Local)

var cardChannelNames = map[string]string{"C0000000001": "dev"}

func cardPermalink(query string) string {
	return fmt.Sprintf("https://example.slack.com/archives/C0000000001/p%d000200%s", cardLinkedAt.Unix(), query)
}

func cardParagraphs(n int) string {
	paragraphs := make([]string, n)
	for i := range paragraphs {
		paragraphs[i] = fmt.Sprintf("Step %d of the rollout went out to the canary pool and held steady for ten minutes before the next one started.", i+1)
	}
	return strings.Join(paragraphs, "\n")
}

func linkedMessageCard(query, body string) blockkit.LegacyAttachment {
	footer := "Slack Conversation"
	if strings.Contains(query, "thread_ts") {
		footer = "Thread in Slack Conversation"
	}
	return blockkit.LegacyAttachment{
		AuthorName: "Claude",
		FromURL:    cardPermalink(query),
		Footer:     footer,
		TS:         cardLinkedAt.Unix(),
		Text:       body,
		Blocks: []blockkit.Block{blockkit.RichTextBlock{Elements: []slack.RichTextElement{
			&slack.RichTextSection{Type: slack.RTESection, Elements: []slack.RichTextSectionElement{
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: "Rollout report. ", Style: &slack.RichTextSectionTextStyle{Bold: true}},
				&slack.RichTextSectionTextElement{Type: slack.RTSEText, Text: body},
			}},
		}}},
	}
}

func cardMessage(ts string, cards ...blockkit.LegacyAttachment) MessageItem {
	return MessageItem{TS: ts, UserID: "U1", UserName: "alice", Timestamp: "4:10 PM", Text: "see the linked thread", LegacyAttachments: cards}
}

func newCardModel(msgs ...MessageItem) *Model {
	m := New(msgs, "general")
	m.SetChannelNames(cardChannelNames)
	return &m
}

func viewPlain(m *Model, height, width int) string {
	return ansi.Strip(m.View(height, width))
}

func TestToggleAttachmentFold(t *testing.T) {
	long := cardMessage("1736093400.000100", linkedMessageCard("?thread_ts=1736000000.000100", cardParagraphs(8)))
	m := newCardModel(long)

	if plain := viewPlain(m, 40, 80); !strings.Contains(plain, "more lines · z to expand") || strings.Contains(plain, "Step 8") {
		t.Fatalf("a long card must start collapsed:\n%s", plain)
	}
	if !m.ToggleAttachmentFold() {
		t.Fatal("ToggleAttachmentFold = false on a message with a long card")
	}
	if plain := viewPlain(m, 40, 80); !strings.Contains(plain, "▾ z to collapse") || !strings.Contains(plain, "Step 8") {
		t.Errorf("expanded card must show the whole body and the collapse row:\n%s", plain)
	}

	m.SetMessages([]MessageItem{long})
	if plain := viewPlain(m, 40, 80); !strings.Contains(plain, "▾ z to collapse") {
		t.Errorf("SetMessages must not fold the card again:\n%s", plain)
	}

	m.ToggleAttachmentFold()
	if plain := viewPlain(m, 40, 80); !strings.Contains(plain, "more lines · z to expand") {
		t.Errorf("second toggle must collapse:\n%s", plain)
	}
}

func TestToggleAttachmentFold_TogglesEveryCardOfTheMessage(t *testing.T) {
	card := linkedMessageCard("", cardParagraphs(8))
	m := newCardModel(cardMessage("1736093400.000100", card, card))
	viewPlain(m, 80, 80)
	m.ToggleAttachmentFold()
	if plain := viewPlain(m, 80, 80); strings.Count(plain, "▾ z to collapse") != 2 {
		t.Errorf("want both cards expanded:\n%s", plain)
	}
}

func TestToggleAttachmentFold_NothingToFold(t *testing.T) {
	m := newCardModel(cardMessage("1736093400.000100", linkedMessageCard("", "short")))
	viewPlain(m, 40, 80)
	before := m.Version()
	if m.ToggleAttachmentFold() || m.Version() != before {
		t.Error("z on a message with no long card must do nothing")
	}
}

// The selected message stays in view when a toggle changes its height.
func TestToggleAttachmentFold_KeepsTheSelectedMessageInView(t *testing.T) {
	var msgs []MessageItem
	for i := 0; i < 6; i++ {
		msgs = append(msgs, MessageItem{TS: fmt.Sprintf("17360930%02d.000100", i), UserID: "U2", UserName: "bob", Timestamp: "4:00 PM", Text: fmt.Sprintf("filler %d", i)})
	}
	msgs = append(msgs, cardMessage("1736093400.000100", linkedMessageCard("", cardParagraphs(30))))
	m := newCardModel(msgs...)
	const height, width = 16, 80

	viewPlain(m, height, width)
	m.ToggleAttachmentFold()
	if plain := viewPlain(m, height, width); !strings.Contains(plain, "alice") || !strings.Contains(plain, "Claude · #dev") {
		t.Errorf("expanded past the pane: the message's top must be in view:\n%s", plain)
	}
	if sel, _ := m.SelectedMessage(); sel.TS != "1736093400.000100" {
		t.Errorf("selection moved to %s on expand", sel.TS)
	}

	m.ToggleAttachmentFold()
	plain := viewPlain(m, height, width)
	if !strings.Contains(plain, "alice") || !strings.Contains(plain, "more lines · z to expand") {
		t.Errorf("collapsed: the whole message must be in view:\n%s", plain)
	}
	if sel, _ := m.SelectedMessage(); sel.TS != "1736093400.000100" {
		t.Errorf("selection moved to %s on collapse", sel.TS)
	}
}

func TestToggleAttachmentFoldAt_ClickOnTheFoldRow(t *testing.T) {
	m := newCardModel(
		cardMessage("1736093400.000100", linkedMessageCard("", cardParagraphs(8))),
		MessageItem{TS: "1736093500.000100", UserID: "U2", UserName: "bob", Timestamp: "4:12 PM", Text: "newest, selected"},
	)
	rowOf := func(plain, needle string) int {
		for y, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, needle) {
				return y
			}
		}
		t.Fatalf("%q not in view:\n%s", needle, plain)
		return -1
	}

	plain := viewPlain(m, 40, 80)
	if m.ToggleAttachmentFoldAt(rowOf(plain, "Step 1 ")) {
		t.Error("a click on a body row must not toggle")
	}
	if !m.ToggleAttachmentFoldAt(rowOf(plain, "z to expand")) {
		t.Fatal("a click on the fold row must toggle")
	}
	plain = viewPlain(m, 40, 80)
	if !strings.Contains(plain, "Step 8") {
		t.Errorf("click did not expand:\n%s", plain)
	}
	if sel, _ := m.SelectedMessage(); sel.TS != "1736093400.000100" {
		t.Errorf("the clicked message must become the selection, got %s", sel.TS)
	}
	if !m.ToggleAttachmentFoldAt(rowOf(plain, "z to collapse")) {
		t.Error("a click on the collapse row must toggle back")
	}
}

// The card wears the selection tint like the rest of the message.
func TestCardCarriesTheSelectionBackground(t *testing.T) {
	styles.Apply("nord", config.Theme{})
	m := newCardModel(cardMessage("1736093400.000100", linkedMessageCard("", cardParagraphs(8))))
	m.buildCache(80)
	selected := strings.Join(m.cache[len(m.cache)-1].linesSelected, "\n")
	if !strings.Contains(selected, "╭") || !strings.Contains(selected, "z to expand") {
		t.Fatalf("no card in the selected rows:\n%s", ansi.Strip(selected))
	}
	if strings.Contains(selected, bgSGRParams(BgANSI())) {
		t.Errorf("selected rows keep the theme background somewhere:\n%q", selected)
	}
	if bare := runsWithoutBackground(selected); len(bare) > 0 {
		t.Errorf("selected rows have runs with no background: %q", bare)
	}
}

// Events drop the cache between draws. z and a click that land before
// the next draw must work the first time.
func TestToggleAttachmentFold_WorksRightAfterTheCacheWasDropped(t *testing.T) {
	long := cardMessage("1736093400.000100", linkedMessageCard("", cardParagraphs(8)))
	foldRow := func(plain, needle string) int {
		for y, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, needle) {
				return y
			}
		}
		t.Fatalf("%q not in view:\n%s", needle, plain)
		return -1
	}

	m := newCardModel(long)
	viewPlain(m, 40, 80)
	m.InvalidateCache()
	if !m.ToggleAttachmentFold() {
		t.Fatal("z after InvalidateCache did nothing")
	}
	plain := viewPlain(m, 40, 80)
	if !strings.Contains(plain, "▾ z to collapse") {
		t.Fatalf("z after InvalidateCache did not expand:\n%s", plain)
	}

	m.InvalidateCache()
	if !m.ToggleAttachmentFoldAt(foldRow(plain, "z to collapse")) {
		t.Fatal("a click after InvalidateCache did nothing")
	}
	plain = viewPlain(m, 40, 80)
	if !strings.Contains(plain, "z to expand") {
		t.Fatalf("a click after InvalidateCache did not collapse:\n%s", plain)
	}

	// A new message changes the message count, which View answers with a
	// full rebuild.
	m.AppendMessage(MessageItem{TS: "1736093500.000100", UserID: "U2", UserName: "bob", Timestamp: "4:12 PM", Text: "newer"})
	if !m.ToggleAttachmentFoldAt(foldRow(plain, "z to expand")) {
		t.Fatal("a click after AppendMessage did nothing")
	}
	if plain := viewPlain(m, 40, 80); !strings.Contains(plain, "▾ z to collapse") {
		t.Fatalf("a click after AppendMessage did not expand:\n%s", plain)
	}
}
