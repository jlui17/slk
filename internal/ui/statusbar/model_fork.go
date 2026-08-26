package statusbar

import (
	"fmt"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/gammons/slk/internal/ui/styles"
)

// SetBootSyncing marks everything on screen for the active workspace as
// cache-served while its network bootstrap is still in flight. Distinct
// from SetSyncing, which covers one channel's background verify fetch;
// the right-side "cached · syncing" pill shows while either is set.
func (m *Model) SetBootSyncing(v bool) {
	if m.bootSyncing != v {
		m.bootSyncing = v
		m.dirty()
	}
}

func (m Model) syncPill() string {
	if !m.syncing && !m.bootSyncing {
		return ""
	}
	return lipgloss.NewStyle().
		Foreground(styles.TextMuted).
		Background(styles.SurfaceDark).
		Render("◌ cached · syncing")
}

// SetReconnectWait switches the connection segment to the reconnect
// countdown. retryAt is the redial deadline; attempt is the 1-based
// retry number (rendered from the second attempt on).
func (m *Model) SetReconnectWait(retryAt time.Time, attempt int) {
	m.connState = StateReconnecting
	m.retryAt = retryAt
	m.attempt = attempt
	m.dirty()
}

// TickReconnect marks the bar dirty so the countdown re-renders, and
// reports whether the segment is still in the reconnect wait — false
// tells the caller to stop its tick chain.
func (m *Model) TickReconnect() bool {
	if m.connState != StateReconnecting {
		return false
	}
	m.dirty()
	return true
}

// formatReconnect renders the reconnect-wait segment: a countdown to
// the redial deadline, plus the attempt number once the first quick
// retry has already failed. A past deadline means the redial is in
// flight ("hello" hasn't arrived yet), rendered without a countdown.
func formatReconnect(retryAt time.Time, attempt int) string {
	label := "⟳ Reconnecting"
	if d := time.Until(retryAt); d > 0 {
		secs := int(d.Round(time.Second) / time.Second)
		if secs < 1 {
			secs = 1
		}
		label = fmt.Sprintf("⟳ Reconnecting in %ds", secs)
	}
	if attempt > 1 {
		label += fmt.Sprintf(" (try %d)", attempt)
	}
	return label
}

// ReconnectTickMsg is the once-a-second tick that refreshes the
// reconnect countdown segment. App starts the chain on the first
// reconnect-wait ConnectionStateMsg and reschedules from the tick
// handler while the segment remains in StateReconnecting.
type ReconnectTickMsg struct{}
