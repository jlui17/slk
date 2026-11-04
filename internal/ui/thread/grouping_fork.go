package thread

import "github.com/gammons/slk/internal/ui/messages"

// The thread pane's grouping (messages/layout_fork.go has the rules).
// A reply's entry records whether it was drawn as a continuation
// (EntryPayload.Continuation); setReplyList draws again any whose
// neighbours now give another answer.

// threadOptions are how the thread pane renders a message: the time in
// the margin (no avatars), no "(edited)", no broadcast label, no reply
// affordance and no sixel rows (plan decision P3). Image hits are
// recorded so a click opens the preview, as in the channel pane.
func threadOptions(isSelected bool) messages.RenderOptions {
	return messages.RenderOptions{IsSelected: isSelected, ImageHits: true, TimeMargin: true}
}

// continues reports whether reply i is drawn as a continuation of
// reply i-1. The first reply starts a group: the replies line sits
// between it and the parent.
func (m *Model) continues(i int) bool {
	return i > 0 && !m.newLineBefore(i) && messages.Continues(m.replies[i-1], m.replies[i])
}

// newLineBefore reports whether the new-messages line sits right above
// reply i: the first reply past the unread boundary.
func (m *Model) newLineBefore(i int) bool {
	b := m.unreadBoundaryTS
	return b != "" && m.replies[i].TS > b && (i == 0 || m.replies[i-1].TS <= b)
}

// visibleReplyCount is the replies the replies line counts: an
// ephemeral reply is not one.
func (m *Model) visibleReplyCount() int {
	n := 0
	for _, r := range m.replies {
		if !r.Ephemeral {
			n++
		}
	}
	return n
}

// regroupReplies draws again each reply whose recorded continuation no
// longer matches its neighbours (a reply came or went next to it, or
// the unread boundary moved). The build's focus colours are kept, as
// refreshStaleReplies keeps them.
func (m *Model) regroupReplies(width int) {
	var wrap *messages.WrapStyles
	for i := range m.replyEntries {
		if i >= len(m.cache) || m.cache[i].Continuation == m.continues(i) {
			continue
		}
		if wrap == nil {
			w := messages.NewWrapStyles(m.marks.builtFocused)
			wrap = &w
		}
		m.replyEntries[i], m.cache[i] = m.renderReplyEntry(i, m.replies[i], width, *wrap)
	}
}
