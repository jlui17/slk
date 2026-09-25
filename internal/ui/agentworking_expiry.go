// Working expires after silence: a working state read from content ("On
// it, will report", an open todo list, an unanswered human message) says
// what the agent meant to do, not that it is still alive. When nothing has
// happened in the thread for agentWorkingExpiry, the state reads idle with
// source working_expired. The rule lives in derived() with the others, so
// a message already older than the period when slk first sees it never
// reads working at all; the tick below only gets the working→idle edge to
// herdr when the period runs out with no event to carry it.
package ui

import (
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// agentWorkingExpiry is provisional: tools/agent-state-report.sh lists the
// expiries the agent then posted past, which is how to tell it is too short.
const agentWorkingExpiry = 10 * time.Minute

const agentWorkingExpiryTickInterval = time.Minute

type agentWorkingExpiryTickMsg struct{}

func agentWorkingExpiryTick() tea.Cmd {
	return tea.Tick(agentWorkingExpiryTickInterval, func(time.Time) tea.Msg {
		return agentWorkingExpiryTickMsg{}
	})
}

// claimAgentWorkingExpiryTick starts the tick chain once, however many
// times herdr reconnects. The hooks that begin a working state return no
// command to schedule a one-shot timer with, so the chain runs for the
// session and each tick is a no-op unless an expiry is due.
func (a *App) claimAgentWorkingExpiryTick() tea.Cmd {
	if a.agentSidebar.expiryTickerOn {
		return nil
	}
	a.agentSidebar.expiryTickerOn = true
	return agentWorkingExpiryTick()
}

// noteActivity records a sign of life in the tracked thread: the newest
// message's own time (so an old message is old from the start), and the
// moment of an edit, the agent's reaction, or a live turn's start or end.
func (g *agentSidebar) noteActivity(at time.Time) {
	if at.After(g.lastActivity) {
		g.lastActivity = at
	}
}

func (g *agentSidebar) workingExpired() bool {
	return !g.lastActivity.IsZero() && g.now().Sub(g.lastActivity) >= agentWorkingExpiry
}

// expireAgentWorking reports the expiry when herdr was last told working
// and silence has since expired it. The tick calls it, and so does every
// event handler before it applies its event: derived() already reads idle
// by then, so without this the handler would compare idle with idle and
// herdr would keep the working it was told.
func (a *App) expireAgentWorking() {
	g := &a.agentSidebar
	if _, source := g.effective(); g.reported == AgentWorking && source == SourceWorkingExpired {
		a.reportAgentThreadState()
	}
}

// slackTSTime is the time a Slack message ts ("1787780670.859699") names,
// to the second. Zero when ts isn't one.
func slackTSTime(ts string) time.Time {
	sec, _, _ := strings.Cut(ts, ".")
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(n, 0)
}
