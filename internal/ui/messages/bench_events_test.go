package messages

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/messages/blockkit"
)

// benchRichMessages returns n messages cycling through the shapes a busy
// channel carries: mrkdwn with links, mentions and emoji, a code block,
// a bulleted list, a long wrapping paragraph, and the Block Kit and
// legacy-attachment fixtures under blockkit/testdata. Every third
// message has reactions and every fifth a thread.
func benchRichMessages(tb testing.TB, n int) []MessageItem {
	tb.Helper()
	type fixture struct {
		blocks []blockkit.Block
		legacy []blockkit.LegacyAttachment
	}
	var fixtures []fixture
	for _, name := range []string{"github_pr.json", "pagerduty_alert.json", "github_pending_review.json", "deploy_approval.json", "oncall_handoff.json", "section_with_fields.json"} {
		data, err := os.ReadFile(filepath.Join("blockkit", "testdata", name))
		if err != nil {
			tb.Fatal(err)
		}
		var p struct {
			Blocks      slack.Blocks       `json:"blocks"`
			Attachments []slack.Attachment `json:"attachments"`
		}
		if err := json.Unmarshal(data, &p); err != nil {
			tb.Fatalf("%s: %v", name, err)
		}
		fixtures = append(fixtures, fixture{blockkit.Parse(p.Blocks), blockkit.ParseAttachments(p.Attachments)})
	}
	texts := []string{
		"Hey <@U2>, the deploy for <https://git.example.com/org/repo/pulls/3478|#3478> is *green* now :tada: and _staging_ looks fine. See <https://grafana.example.com/d/abc?from=now-1h|the dashboard> for p99; `checkout-svc` is back under SLO.",
		"Here is the fix:\n```\nfunc (m *Model) AppendMessage(msg MessageItem) {\n\tm.messages = append(m.messages, msg)\n\tm.dirty()\n}\n```\nand the test passes.",
		"Plan:\n• Merge <https://example.com/pr/1|PR 1> once CI is green\n• Rebase <https://example.com/pr/2|PR 2> on main\n• Ping <@U3> for review :eyes:\n• Ship it",
		strings.Repeat("A longer message that wraps several times across the pane, with a link <https://docs.example.com/some/long/path|docs> and some ~struck~ text and *bold* words. ", 5),
	}
	msgs := make([]MessageItem, n)
	for i := range msgs {
		msg := MessageItem{
			TS:        fmt.Sprintf("%d.000100", 1700000000+i),
			UserName:  []string{"alice", "bob", "carol", "deploybot"}[i%4],
			UserID:    fmt.Sprintf("U%d", i%4+1),
			Timestamp: "10:30 AM",
		}
		if k := i % 10; k < len(texts) {
			msg.Text = texts[k]
		} else {
			f := fixtures[k-len(texts)]
			msg.Blocks, msg.LegacyAttachments = f.blocks, f.legacy
			msg.Text = "fallback text"
		}
		if i%3 == 0 {
			msg.Reactions = []ReactionItem{
				{Emoji: "thumbsup", Count: 2, UserIDs: []string{"U1", "U2"}},
				{Emoji: "eyes", Count: 1, UserIDs: []string{"U3"}},
			}
		}
		if i%5 == 0 {
			msg.ReplyCount = 3
		}
		msgs[i] = msg
	}
	return msgs
}

const benchEventH, benchEventW = 60, 160

// BenchmarkViewSetMessages is opening a channel: every message rendered.
func BenchmarkViewSetMessages(b *testing.B) {
	msgs := benchRichMessages(b, 200)
	m := New(nil, "general")
	_ = m.View(benchEventH, benchEventW)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.SetMessages(msgs)
		_ = m.View(benchEventH, benchEventW)
	}
}

// BenchmarkViewReaction is a reaction added to or removed from a message
// in the open channel, then the frame that shows it.
func BenchmarkViewReaction(b *testing.B) {
	msgs := benchRichMessages(b, 200)
	m := New(msgs, "general")
	_ = m.View(benchEventH, benchEventW)
	ts := msgs[150].TS

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.UpdateReaction(ts, "rocket", "U9", i%2 == 1)
		_ = m.View(benchEventH, benchEventW)
	}
}

// BenchmarkViewEdit is a message_changed for a message in the open
// channel, then the frame that shows it.
func BenchmarkViewEdit(b *testing.B) {
	msgs := benchRichMessages(b, 200)
	m := New(msgs, "general")
	_ = m.View(benchEventH, benchEventW)
	edits := []MessageItem{
		{TS: msgs[150].TS, Text: "edited once", IsEdited: true},
		{TS: msgs[150].TS, Text: "edited twice, now a little longer", IsEdited: true},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.UpdateMessageInPlace(edits[i%2])
		_ = m.View(benchEventH, benchEventW)
	}
}

// BenchmarkViewAppend is a new message in the open channel, then the
// frame that shows it. The channel is reset to 200 messages every 100
// appends, off the clock.
func BenchmarkViewAppend(b *testing.B) {
	base := benchRichMessages(b, 200)
	extra := benchRichMessages(b, 10)
	m := New(nil, "general")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%100 == 0 {
			b.StopTimer()
			m.SetMessages(append([]MessageItem(nil), base...))
			_ = m.View(benchEventH, benchEventW)
			b.StartTimer()
		}
		msg := extra[i%len(extra)]
		msg.TS = fmt.Sprintf("%d.000100", 1800000000+i)
		m.AppendMessage(msg)
		_ = m.View(benchEventH, benchEventW)
	}
}

// BenchmarkViewRemove is a message deleted from the middle of the open
// channel, then the next frame. The channel is reset to 200 messages
// every 100 deletes, off the clock.
func BenchmarkViewRemove(b *testing.B) {
	base := benchRichMessages(b, 200)
	m := New(nil, "general")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%100 == 0 {
			b.StopTimer()
			m.SetMessages(append([]MessageItem(nil), base...))
			_ = m.View(benchEventH, benchEventW)
			b.StartTimer()
		}
		m.RemoveMessageByTS(m.Messages()[50].TS)
		_ = m.View(benchEventH, benchEventW)
	}
}
