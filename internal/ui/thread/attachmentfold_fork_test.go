package thread

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// A synthetic stand-in with the shape of a Slack message unfurl.
func linkedMessageCard(lines int) blockkit.LegacyAttachment {
	body := make([]string, lines)
	for i := range body {
		body[i] = fmt.Sprintf("linked line %d", i+1)
	}
	return blockkit.LegacyAttachment{
		AuthorName: "Claude",
		FromURL:    "https://example.slack.com/archives/C0000000001/p1736092920000200?thread_ts=1736000000.000100&cid=C0000000001",
		Footer:     "Thread in Slack Conversation",
		Text:       strings.Join(body, "\n"),
	}
}

func cardThread() *Model {
	m := New()
	m.SetChannelNames(map[string]string{"C0000000001": "dev"})
	parent := messages.MessageItem{TS: "1.0", UserName: "alice", Text: "parent", LegacyAttachments: []blockkit.LegacyAttachment{linkedMessageCard(8)}}
	replies := []messages.MessageItem{
		{TS: "1.001", UserName: "bob", Text: "no card here"},
		{TS: "1.002", UserName: "bob", Text: "see the linked thread", LegacyAttachments: []blockkit.LegacyAttachment{linkedMessageCard(30)}},
	}
	m.SetThread(parent, replies, "C1", "1.0")
	return m
}

func plainView(m *Model, height int) string { return ansi.Strip(m.View(height, 80)) }

func TestToggleAttachmentFold_Reply(t *testing.T) {
	m := cardThread() // the newest reply is selected
	const height = 14

	if plain := plainView(m, height); !strings.Contains(plain, "▸ 27 more lines · z to expand") || !strings.Contains(plain, "see the linked thread") {
		t.Fatalf("the reply's card must start collapsed:\n%s", plain)
	}
	if !m.ToggleAttachmentFold() {
		t.Fatal("ToggleAttachmentFold = false on a reply with a long card")
	}
	plain := plainView(m, height)
	if !strings.Contains(plain, "see the linked thread") || !strings.Contains(plain, "Claude · Thread in #dev") || !strings.Contains(plain, "linked line 4") {
		t.Errorf("expanded past the pane: the reply's top must be in view:\n%s", plain)
	}
	if got := m.SelectedReply(); got == nil || got.TS != "1.002" {
		t.Errorf("selection moved on expand: %+v", got)
	}

	m.ToggleAttachmentFold()
	if plain := plainView(m, height); !strings.Contains(plain, "see the linked thread") || !strings.Contains(plain, "▸ 27 more lines · z to expand") {
		t.Errorf("collapsed: the whole reply must be in view:\n%s", plain)
	}
}

func TestToggleAttachmentFold_ParentAndStateSurvivesSetThread(t *testing.T) {
	m := cardThread()
	plainView(m, 60)
	m.MoveUp()
	m.MoveUp()
	m.MoveUp() // the parent
	plainView(m, 60)
	if !m.ToggleAttachmentFold() {
		t.Fatal("ToggleAttachmentFold = false on a parent with a long card")
	}
	if plain := plainView(m, 60); !strings.Contains(plain, "linked line 8") || strings.Count(plain, "▾ z to collapse") != 1 {
		t.Errorf("parent card must expand, the reply's stay folded:\n%s", plain)
	}

	m.SetThread(m.ParentMsg(), m.Replies(), "C1", "1.0")
	if plain := plainView(m, 60); !strings.Contains(plain, "▾ z to collapse") {
		t.Errorf("SetThread must not fold the card again:\n%s", plain)
	}
}

func TestToggleAttachmentFold_NothingToFold(t *testing.T) {
	m := cardThread()
	plainView(m, 60)
	m.MoveUp() // "no card here"
	plainView(m, 60)
	before := m.Version()
	if m.ToggleAttachmentFold() || m.Version() != before {
		t.Error("z on a reply with no long card must do nothing")
	}
}

func TestToggleAttachmentFoldAt_ClickOnTheFoldRow(t *testing.T) {
	m := cardThread()
	plain := plainView(m, 60)
	foldRows := func(plain string) (rows []int) {
		for y, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, "z to expand") {
				rows = append(rows, y)
			}
		}
		return rows
	}
	rows := foldRows(plain)
	if len(rows) != 2 {
		t.Fatalf("want the parent's and the reply's fold rows in view:\n%s", plain)
	}
	if m.ToggleAttachmentFoldAt(rows[0] - 1) {
		t.Error("a click on a body row must not toggle")
	}
	if !m.ToggleAttachmentFoldAt(rows[0]) {
		t.Fatal("a click on the parent's fold row must toggle")
	}
	plain = plainView(m, 60)
	if !strings.Contains(plain, "linked line 8") || len(foldRows(plain)) != 1 {
		t.Errorf("only the parent's card must expand:\n%s", plain)
	}
	if got := m.SelectedReply(); got == nil || got.TS != "1.0" {
		t.Errorf("the clicked message must become the selection, got %+v", got)
	}
}

// Events drop the cache between draws. z and a click that land before
// the next draw must work the first time.
func TestToggleAttachmentFold_WorksRightAfterTheCacheWasDropped(t *testing.T) {
	m := cardThread() // the newest reply is selected
	plain := plainView(m, 60)
	m.InvalidateCache()
	if !m.ToggleAttachmentFold() {
		t.Fatal("z on a reply after InvalidateCache did nothing")
	}
	if plain = plainView(m, 60); !strings.Contains(plain, "linked line 30") {
		t.Fatalf("z on a reply after InvalidateCache did not expand:\n%s", plain)
	}

	parentFoldRow := -1
	for y, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, "▸ 5 more lines") {
			parentFoldRow = y
		}
	}
	m.InvalidateCache()
	if !m.ToggleAttachmentFoldAt(parentFoldRow) {
		t.Fatalf("a click on the parent's fold row (row %d) after InvalidateCache did nothing:\n%s", parentFoldRow, plain)
	}
	if plain = plainView(m, 60); !strings.Contains(plain, "linked line 8") {
		t.Fatalf("the click did not expand the parent's card:\n%s", plain)
	}

	m.InvalidateCache()
	if !m.ToggleAttachmentFold() { // the click selected the parent
		t.Fatal("z on the parent after InvalidateCache did nothing")
	}
	if plain = plainView(m, 60); !strings.Contains(plain, "▸ 5 more lines") {
		t.Fatalf("z on the parent after InvalidateCache did not collapse:\n%s", plain)
	}
}
