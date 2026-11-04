package thread

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	emojiutil "github.com/gammons/slk/internal/emoji"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
)

// renderThreadHeader renders the thread header row at exactly width
// cells, as Slack's thread view heads it:
//
//	# general › Thread                                esc close
//
// The author and the reply count are left to the parent and the
// replies line under it. When width is short the hint drops first, then
// the channel is truncated with "…", then dropped; "Thread" is kept. It
// returns the channel label's width, the click target that goes back
// to the channel (HitTestBreadcrumbChannel).
func renderThreadHeader(width int, channelName, channelType string) (string, int) {
	if width <= 0 {
		return "", 0
	}
	const (
		sep    = " › "
		thread = "Thread"
	)
	channel := ""
	if channelName != "" {
		channel = messages.ChannelGlyph(channelType) + " " + channelName
	}
	leftWidth := func() int {
		if channel == "" {
			return len(thread)
		}
		return emojiutil.Width(channel+sep) + len(thread)
	}
	showHint := leftWidth()+breadcrumbHintGap+len(breadcrumbHint) <= width
	if channel != "" && leftWidth() > width {
		room := width - emojiutil.Width(sep+thread)
		if room < 2 {
			channel = ""
		} else {
			channel = ansi.Truncate(channel, room, "…")
		}
	}
	channelCols := emojiutil.Width(channel)

	bg := lipgloss.NewStyle().Background(styles.Background)
	var b strings.Builder
	if channel != "" {
		b.WriteString(bg.Foreground(styles.TextMuted).Render(channel + sep))
	}
	b.WriteString(bg.Foreground(styles.TextPrimary).Bold(true).Render(thread))
	left := b.String()
	if emojiutil.Width(left) > width {
		left = ansi.Truncate(left, width, "")
	}
	used := emojiutil.Width(left)
	if !showHint {
		return left + bg.Render(strings.Repeat(" ", width-used)), channelCols
	}
	gap := width - used - len(breadcrumbHint)
	return left + bg.Render(strings.Repeat(" ", gap)) +
		bg.Foreground(styles.TextMuted).Render(breadcrumbHint), channelCols
}
