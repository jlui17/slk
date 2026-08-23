package ui

import (
	tea "charm.land/bubbletea/v2"
)

// cmdReload forces every workspace's websocket to reconnect — the
// :command alias of the ctrl+r manual reload.
func cmdReload(a *App, _ []string) tea.Cmd { return a.reloadConnections() }
