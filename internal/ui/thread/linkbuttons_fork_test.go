package thread

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/messages/blockkit/blockkittest"
)

// The thread panel draws an ephemeral reply like the pane: "only visible
// to you" on its header row, "o to open" under its link button.
func TestRenderThreadMessage_ColonyEphemeral(t *testing.T) {
	m := New()
	msg := messages.MessageItem{
		TS:          "1787400000.000200",
		ThreadTS:    "1787300000.000100",
		UserName:    "Colony",
		Timestamp:   "3:04 PM",
		Text:        "Your annotation was sent.",
		Blocks:      blockkittest.ColonyEphemeral("https://x.slack.com/archives/C1/p1", "https://x.slack.com/archives/C1/p1787300000000100"),
		IsEphemeral: true,
	}
	got, _, _ := m.renderThreadMessage(msg, 100, nil, nil, false)
	plain := ansi.Strip(got)
	if header := strings.Split(plain, "\n")[0]; !strings.Contains(header, "only visible to you") {
		t.Errorf("header row = %q, want \"only visible to you\"", header)
	}
	if !strings.Contains(plain, "o to open") || strings.Contains(plain, "open in Slack to interact") {
		t.Errorf("want the \"o to open\" hint alone, got %q", plain)
	}
}
