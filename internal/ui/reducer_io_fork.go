package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/ui/statusbar"
)

// reconnectTick schedules the next reconnect-countdown refresh.
func reconnectTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return statusbar.ReconnectTickMsg{}
	})
}

// connectionState is one workspace's cached connection state; retryAt
// and attempt are meaningful only in StateReconnecting.
type connectionState struct {
	state   statusbar.ConnectionState
	retryAt time.Time
	attempt int
}

// storeConnState caches teamID's latest connection state.
func (a *App) storeConnState(teamID string, st connectionState) {
	if a.connStates == nil {
		a.connStates = map[string]connectionState{}
	}
	a.connStates[teamID] = st
}

// applyConnState pushes a connection state into the statusbar segment
// and starts the countdown tick chain when entering a reconnect wait
// (one chain at a time; a live chain just picks up the new values).
func (a *App) applyConnState(st connectionState) tea.Cmd {
	if st.state == statusbar.StateReconnecting {
		a.statusbar.SetReconnectWait(st.retryAt, st.attempt)
		if !a.claimReconnectTicker() {
			return nil
		}
		return reconnectTick()
	}
	a.statusbar.SetConnectionState(st.state)
	return nil
}

// claimReconnectTicker / clearReconnectTicker guard the reconnect
// countdown tick chain, mirroring presenceController.ClaimTicker /
// ClearTicker for the DND chain: Claim returns true exactly once
// until Clear, so rapid reconnect-wait messages can't accumulate
// parallel chains.
func (a *App) claimReconnectTicker() bool {
	if a.reconnectTickerOn {
		return false
	}
	a.reconnectTickerOn = true
	return true
}

func (a *App) clearReconnectTicker() { a.reconnectTickerOn = false }

// applyActiveConnState pushes the active workspace's cached connection
// state on workspace switch; a workspace with no cached state yet has
// never connected, which is StateConnecting.
func (a *App) applyActiveConnState() tea.Cmd {
	st, ok := a.connStates[a.activeTeamID]
	if !ok {
		st = connectionState{state: statusbar.StateConnecting}
	}
	return a.applyConnState(st)
}

// SetReloader sets the callback that forces every workspace's
// websocket to reconnect (ctrl+r / :reload).
func (a *App) SetReloader(fn ReloadFunc) {
	a.reloader = fn
}

// reloadConnections runs the manual reload and acknowledges it with a
// toast. No-op before SetReloader is wired.
func (a *App) reloadConnections() tea.Cmd {
	if a.reloader == nil {
		return nil
	}
	a.reloader()
	cmds := []tea.Cmd{toastWithClear(a, "Reloading connections…", 2*time.Second)}
	if refetch := a.refetchOpenThreadCmd(); refetch != nil {
		cmds = append(cmds, refetch)
	}
	return tea.Batch(cmds...)
}

// refetchOpenThreadCmd returns a cmd that refetches the open thread
// panel's replies from Slack, or nil when no thread is open. Reload
// needs this because a thread reply swallowed by a websocket gap is
// unrecoverable by reconnect catch-up alone: the channel-level backfill
// reads conversations.history, which never returns thread replies, and
// the thread catch-up writes only to the cache, which an open panel
// never re-reads.
func (a *App) refetchOpenThreadCmd() tea.Cmd {
	if !a.threadVisible || a.threadPanel.ThreadTS() == "" {
		return nil
	}
	threads := a.threads
	chID := ids.ChannelID(a.threadPanel.ChannelID())
	threadTS := ids.ThreadTS(a.threadPanel.ThreadTS())
	return func() tea.Msg { return threads.Fetch(chID, threadTS) }
}

// threadRefetchOnReconnect returns the open-thread refetch cmd when a
// workspace's connection state transitions back to Connected, or nil.
// Must run before the caller stores the new state in connStates: the
// transition is detected against the previously cached state. A first
// connect (no cached entry) doesn't refetch — the boot thread-open
// path fetches on its own — and only the active workspace qualifies,
// since the thread panel shows a thread of the active workspace.
func (a *App) threadRefetchOnReconnect(teamID string, newState statusbar.ConnectionState) tea.Cmd {
	if newState != statusbar.StateConnected || teamID == "" || teamID != a.activeTeamID {
		return nil
	}
	prev, ok := a.connStates[teamID]
	if !ok || prev.state == statusbar.StateConnected {
		return nil
	}
	return a.refetchOpenThreadCmd()
}
