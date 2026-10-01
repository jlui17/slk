package thread

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// The text spells the permalink with &amp;, as Slack's mrkdwn does; the
// card's from_url spells it plain.
func TestPermalinkChip_ParentAndReply(t *testing.T) {
	text := "see <https://example.slack.com/archives/C0000000001/p1736092920000200?thread_ts=1736000000.000100&amp;cid=C0000000001>"
	card := []blockkit.LegacyAttachment{linkedMessageCard(2)}
	m := New()
	m.SetChannelNames(map[string]string{"C0000000001": "dev"})
	m.SetThread(
		messages.MessageItem{TS: "1.0", UserName: "alice", Text: text, LegacyAttachments: card},
		[]messages.MessageItem{
			{TS: "1.001", UserName: "bob", Text: text, LegacyAttachments: card},
			{TS: "1.002", UserName: "bob", Text: text},
		}, "C1", "1.0")

	plain := plainView(m, 60)
	if got := strings.Count(plain, "see ↳ Claude in #dev"); got != 2 {
		t.Errorf("want the chip in the parent and in the carded reply, got %d:\n%s", got, plain)
	}
	if !strings.Contains(plain, "▌https://example.slack.com/archives/C0000000001/p1736092920000200") {
		t.Errorf("a permalink with no card must stay a full URL:\n%s", plain)
	}
}
