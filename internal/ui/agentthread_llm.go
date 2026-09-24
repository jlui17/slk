package ui

import (
	"strings"

	"github.com/gammons/slk/internal/ui/messages"
)

// maybeRequestAgentTabLabel fires the tracked thread's one automatic model
// label: the :retitle request (same transcript, same generator, same
// reducer), sent the first time a replies load lands for the tracked
// thread. Every open path paints the panel root-only (nil replies) and the
// load follows one cache read or fetch later, so waiting for it costs a
// moment under the deterministic label and lets the model read the thread
// rather than the root alone; a thread with no replies yet loads an empty,
// non-nil page and sends its root. Silent where :retitle toasts: with
// nothing configured or nothing to send, the deterministic label stands.
// The id hoisted from the root rides along as the fallback for a model
// that answers none.
func (a *App) maybeRequestAgentTabLabel(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) {
	if a.agentSidebar.relabelGen == nil || a.agentSidebar.nameTab == nil ||
		a.agentSidebar.labelRequested || replies == nil || !a.tracksThread("", channelID, threadTS) {
		return
	}
	t := a.agentSidebar.thread
	transcript := a.retitleTranscript(parent, replies, t.botUserID)
	if transcript == "" {
		return
	}
	a.agentSidebar.labelRequested = true
	root := a.flattenRootText(stripMention(parent.Text, t.botUserID))
	a.agentSidebar.relabelGen(t.teamID, t.channelID, t.threadTS, transcript, hoistTaskID(root))
}

// sanitizeModelLabel normalizes a model completion into tab-label shape:
// first line only, surrounding quotes and an echo of taskID (the known
// id for the label) removed, whitespace collapsed, truncated like
// the deterministic label. Only that known id is stripped — a taskIDRe
// sweep would also eat hyphen-digit terms the model wrote ("utf-8",
// "sha-256"). Empty means unusable — the caller keeps the label already
// on the tab.
func sanitizeModelLabel(raw, taskID string) string {
	s := raw
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, "\"'`“”‘’")
	if taskID != "" {
		s = strings.Replace(s, taskID, "", 1)
	}
	return normalizeTabSnippet(s)
}
