// The :agent mark: the user names the open thread an agent thread when
// detection misses it, as when the agent is first mentioned in a reply
// partway down. A marked thread is detected from its replies as well as
// its root, on this open and every later one, restarts included.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ui/messages"
)

// AgentThreadMarkStore persists the threads marked with :agent, per
// workspace. The implementation writes storage.
type AgentThreadMarkStore interface {
	AgentThreadMarked(workspaceID, channelID, threadTS string) (bool, error)
	MarkAgentThread(workspaceID, channelID, threadTS string) error
	UnmarkAgentThread(workspaceID, channelID, threadTS string) error
}

// SetAgentThreadMarks installs the :agent mark store. Unset, :agent
// reports the agent sidebar unavailable and no thread reads as marked.
func (a *App) SetAgentThreadMarks(marks AgentThreadMarkStore) {
	a.agentSidebar.marks = marks
}

func init() { commands["agent"] = cmdAgent }

// cmdAgent marks the thread open in the thread panel as an agent thread
// and tracks it at once, as if detection had found it; on a marked thread
// it removes the mark. Unmarking leaves the thread tracked until another
// agent thread replaces it, the rule every tracked thread follows; the
// mark only decides what later opens detect.
func cmdAgent(a *App, _ []string) tea.Cmd {
	if a.agentSidebar.report == nil || a.agentSidebar.userInfo == nil || a.agentSidebar.marks == nil {
		return toastWithClear(a, "Agent threads need slk in a herdr pane", 2*time.Second)
	}
	channelID, threadTS := a.threadPanel.ChannelID(), a.threadPanel.ThreadTS()
	if threadTS == "" {
		return toastWithClear(a, "Open a thread first", 2*time.Second)
	}
	marked, err := a.agentSidebar.marks.AgentThreadMarked(a.activeTeamID, channelID, threadTS)
	if err != nil {
		return toastWithClear(a, "Reading agent mark failed: "+err.Error(), 2*time.Second)
	}
	if marked {
		if err := a.agentSidebar.marks.UnmarkAgentThread(a.activeTeamID, channelID, threadTS); err != nil {
			return toastWithClear(a, "Unmarking failed: "+err.Error(), 2*time.Second)
		}
		return toastWithClear(a, "Unmarked agent thread", 2*time.Second)
	}
	parent := a.threadPanel.ParentMsg()
	replies := a.threadPanel.Replies()
	botUserID, name, ok := a.threadBot(parent, replies)
	if !ok {
		return toastWithClear(a, "No bot in this thread", 2*time.Second)
	}
	if err := a.agentSidebar.marks.MarkAgentThread(a.activeTeamID, channelID, threadTS); err != nil {
		return toastWithClear(a, "Marking failed: "+err.Error(), 2*time.Second)
	}
	// The open path's agent steps, with the bot already found.
	a.trackAgentThread(parent, channelID, threadTS, botUserID, name)
	a.snapshotAgentThreadLast(parent, replies, channelID, threadTS)
	return tea.Batch(a.maybeRequestAgentTabLabel(parent, replies, channelID, threadTS),
		toastWithClear(a, "Marked agent thread (@"+name+")", 2*time.Second))
}

// agentThreadMarked reports whether the active workspace's thread carries
// an :agent mark. A failed read counts as unmarked.
func (a *App) agentThreadMarked(channelID, threadTS string) bool {
	if a.agentSidebar.marks == nil {
		return false
	}
	marked, err := a.agentSidebar.marks.AgentThreadMarked(a.activeTeamID, channelID, threadTS)
	if err != nil {
		debuglog.General("agent mark %s/%s: %v", channelID, threadTS, err)
		return false
	}
	return marked
}

// threadBot finds a marked thread's agent: the first bot mentioned in, or
// writing, the root and then each loaded reply in order. Ephemerals are
// skipped: the thread panel holds them, but detection's reloads and a
// restart don't, so a bot found in one would change the thread's agent.
func (a *App) threadBot(parent messages.MessageItem, replies []messages.MessageItem) (userID, name string, ok bool) {
	for _, m := range append([]messages.MessageItem{parent}, replies...) {
		if m.IsEphemeral {
			continue
		}
		if userID, name, ok = a.firstBotMention(m.Text); ok {
			return userID, name, true
		}
		if userID, name, ok = a.botUser(m.UserID); ok {
			return userID, name, true
		}
	}
	return "", "", false
}
