package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

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
