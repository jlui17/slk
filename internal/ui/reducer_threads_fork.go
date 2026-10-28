package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/messages"
)

// markFetchedThreadRead marks the open thread read up to the newest of
// the fetched replies when the user can see them: the pane's herdr tab
// is in view and the terminal has focus. Otherwise the mark is staged,
// as an arriving reply's is, and the refocus flush sends it. Not every
// fetch is one the user asked for: the refetch on reconnect after a
// laptop wake lands in every pane with a thread open, and Slack marks
// read only what is viewed.
func (a *App) markFetchedThreadRead(channelID, threadTS string, replies []messages.MessageItem) tea.Cmd {
	if !a.PaneViewed() || !a.terminalFocused {
		latestTS := threadTS
		if t := newestMarkableReplyTS(replies); t != "" {
			latestTS = t
		}
		a.recordThreadMark(channelID, threadTS, latestTS)
		return nil
	}
	cmd := a.markOpenThreadRead(channelID, threadTS, replies)
	a.markAgentThreadRead(a.activeTeamID, channelID, threadTS)
	return cmd
}
