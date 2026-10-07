package messages

import (
	"slices"
	"strings"

	"github.com/gammons/slk/internal/ui/styles"
)

// EphemeralMark ends the header row of a message Slack shows to the user
// alone, saying so; the text carries the meaning, not the style.
func EphemeralMark(msg MessageItem) string {
	if !msg.IsEphemeral {
		return ""
	}
	return " " + styles.Timestamp.Render("only visible to you")
}

// oldestNonEphemeralTS is the ts of the oldest message of msgs that is
// not an ephemeral, "" when there is none. History paging anchors there:
// an ephemeral can be older than every loaded message, and anchoring on
// it would skip the history between the two.
func oldestNonEphemeralTS(msgs []MessageItem) string {
	for _, m := range msgs {
		if !m.IsEphemeral {
			return m.TS
		}
	}
	return ""
}

// prependOlder puts older, a page of history older than the oldest
// message that is not an ephemeral, in front of the messages, with the
// ephemerals ahead of that message laid among older by ts. The
// selection stays on its message.
func (m *Model) prependOlder(older []MessageItem) {
	lead := slices.IndexFunc(m.messages, func(msg MessageItem) bool { return !msg.IsEphemeral })
	if lead < 0 {
		lead = len(m.messages)
	}
	selectedTS := ""
	if m.selected >= 0 && m.selected < lead {
		selectedTS = m.messages[m.selected].TS
	}
	m.messages = append(older, m.messages...)
	slices.SortStableFunc(m.messages[:len(older)+lead], func(a, b MessageItem) int { return strings.Compare(a.TS, b.TS) })
	m.selected += len(older)
	if selectedTS != "" {
		m.selected = slices.IndexFunc(m.messages, func(msg MessageItem) bool { return msg.TS == selectedTS })
	}
}

// ReplaceEphemeral puts msg, an ephemeral its app replaced, in place of
// the message with its ts, whole, as the next reload lays it in: Slack
// marks no ephemeral edited. It reports whether the message was there.
func (m *Model) ReplaceEphemeral(msg MessageItem) bool {
	i := slices.IndexFunc(m.messages, func(old MessageItem) bool { return old.TS == msg.TS })
	if i < 0 {
		return false
	}
	m.messages[i] = msg
	m.cache = nil
	m.dirty()
	return true
}
