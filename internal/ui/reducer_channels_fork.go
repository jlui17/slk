package ui

import "github.com/gammons/slk/internal/ui/messages"

// loadedLastRead is the last-read boundary a pane takes from a
// MessagesLoadedMsg: its LastReadTS, except on the reconnect refresh
// (KeepLastRead), where a pane that has a boundary keeps it, and with
// it the "── new ──" line, as Slack keeps the new-messages line while
// the channel is on screen. By the time of a reconnect, opening the
// channel has marked it read to its newest message, so LastReadTS would
// drop the line.
func loadedLastRead(mm *messages.Model, m MessagesLoadedMsg) string {
	if m.KeepLastRead && mm.LastReadTS() != "" {
		return mm.LastReadTS()
	}
	return m.LastReadTS
}
