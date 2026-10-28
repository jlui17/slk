package ui

import (
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/muesli/reflow/truncate"

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
// that answers none. A !review-open thread's request waits for the
// returned fetch (agentthread_reviewopen.go).
func (a *App) maybeRequestAgentTabLabel(parent messages.MessageItem, replies []messages.MessageItem, channelID, threadTS string) tea.Cmd {
	if a.agentSidebar.requestLabel == nil || a.agentSidebar.nameTab == nil ||
		a.agentSidebar.labelRequested || replies == nil || !a.tracksThread("", channelID, threadTS) {
		return nil
	}
	t := a.agentSidebar.thread
	transcript := a.retitleTranscript(parent, replies, t.botUserID, "")
	if transcript == "" {
		return nil
	}
	a.agentSidebar.labelRequested = true
	root := a.flattenMrkdwn(stripMention(parent.Text, t.botUserID))
	target := labelTarget{teamID: t.teamID, channelID: t.channelID, threadTS: t.threadTS, botUserID: t.botUserID}
	return a.requestAgentTabLabel(target, parent, replies, transcript, hoistTaskID(root), 0)
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

// taskIDRe matches tracker-style task ids ("colony-562", "TAIGA-41"). Two+
// letters before the dash keeps single-letter false positives out; two+
// digits after it keep org names ("colony-2's twin") from reading as ids,
// at the cost of never hoisting a genuine single-digit id. Short hyphenated
// terms with digits ("sha-256") can still match, and the first occurrence
// wins because task ids conventionally lead the message.
var taskIDRe = regexp.MustCompile(`\b[A-Za-z]{2,}-\d{2,}\b`)

// prRefRe matches "#1164"-style PR/issue refs, the id a review-thread root
// carries when it has no tracker task id. Two+ digits so prose ordinals
// ("attempt #1") don't read as refs.
var prRefRe = regexp.MustCompile(`#\d{2,}\b`)

// urlRe matches a URL so hoisting can ignore it: a pasted GitHub link's
// "#issuecomment-15686" fragment matches taskIDRe and is never the
// thread's task.
var urlRe = regexp.MustCompile(`https?://\S+`)

// hoistTaskID picks the id a tab label hoists to its "[id]" prefix: the
// first tracker task id, else the first PR/issue ref, URLs excluded.
// Empty when the text carries neither.
func hoistTaskID(flat string) string {
	flat = urlRe.ReplaceAllString(flat, "")
	if id := taskIDRe.FindString(flat); id != "" {
		return id
	}
	return prRefRe.FindString(flat)
}

// strayPunctRe matches punctuation left dangling between spaces once the
// task id is lifted out of the middle of a sentence.
var strayPunctRe = regexp.MustCompile(`\s+[:,;.\-]+(\s|$)`)

// maxTabLabel caps the label snippet on both the deterministic and model
// paths; a "[colony-562] " task-id prefix rides on top, so a full label
// can exceed it.
const maxTabLabel = 30

// agentTabLabel derives a short tab name from the flattened root text with
// the agent's own mention already stripped (the tab bar has no room for the
// part every agent thread shares). A task id or PR ref anywhere in the text
// is hoisted to a leading "[colony-562] " / "[#1164] " prefix, matching the
// workspace-labeling convention, and removed from the snippet so it doesn't
// appear twice.
func agentTabLabel(flat string) string {
	label := strings.TrimLeft(strings.TrimSpace(flat), ":,;.- ")
	if label == "" {
		label = flat
	}
	if id := hoistTaskID(label); id != "" {
		return withTaskID(id, normalizeTabSnippet(strings.Replace(label, id, "", 1)))
	}
	return truncate.StringWithTail(label, maxTabLabel, "…")
}

// normalizeTabSnippet collapses the gap a removed task id leaves (lifting
// "slk-373" out of "traffic for slk-373, the fix" would strand "traffic
// for , the fix"), trims dangling punctuation, and truncates to
// maxTabLabel. Shared by the deterministic and model label paths so their
// rendering can't drift.
func normalizeTabSnippet(s string) string {
	s = strayPunctRe.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimLeft(s, ":,;.- ")
	s = strings.TrimRight(s, ":,;- ")
	return truncate.StringWithTail(s, maxTabLabel, "…")
}

// withTaskID renders the "[id] snippet" tab-label shape, "[id]" alone when
// the snippet is empty.
func withTaskID(id, snippet string) string {
	if snippet == "" {
		return "[" + id + "]"
	}
	return "[" + id + "] " + snippet
}
