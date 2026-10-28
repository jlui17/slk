package messages

import (
	"github.com/gammons/slk/internal/ui/messages/blockkit"
	"github.com/gammons/slk/internal/ui/styles"
)

// EphemeralLabel is the marker row drawn above the author line of a
// message only the current user can see. Shared by the messages and
// thread panes so the two cannot drift.
func EphemeralLabel() string {
	return styles.Timestamp.Render("Only visible to you")
}

// ButtonAction is one pressable legacy button and the attachment it is on.
type ButtonAction struct {
	Attachment blockkit.LegacyAttachment
	Action     blockkit.LegacyAction
}

// ButtonActions returns msg's legacy buttons in order. Select menus are not
// buttons and are skipped.
func ButtonActions(msg MessageItem) []ButtonAction {
	var out []ButtonAction
	for _, att := range msg.LegacyAttachments {
		for _, act := range att.Actions {
			if act.Type == "button" {
				out = append(out, ButtonAction{Attachment: att, Action: act})
			}
		}
	}
	return out
}

// HasButtonActions reports whether msg has a legacy button to press.
func HasButtonActions(msg MessageItem) bool {
	for _, att := range msg.LegacyAttachments {
		for _, act := range att.Actions {
			if act.Type == "button" {
				return true
			}
		}
	}
	return false
}

// InteractiveHint is the line under a message with interactive elements:
// an ephemeral with buttons can be answered in slk with b; anything else
// still needs Slack.
func InteractiveHint(msg MessageItem) string {
	if msg.Ephemeral && HasButtonActions(msg) {
		return "b to respond"
	}
	return "↗ open in Slack to interact"
}
