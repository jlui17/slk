package main

import (
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/herdr"
	"github.com/gammons/slk/internal/ui"
)

// wireHerdr connects app's agent-thread reporting to the herdr pane
// slk is running in, when there is one. The returned close func
// releases the pane's sidebar entry and must be deferred by the
// caller; it is nil when herdr is absent or disabled.
func wireHerdr(app *ui.App, db *cache.DB, disabled bool) func() {
	hr := herdr.NewReporterFromEnv()
	if hr == nil || disabled {
		return nil
	}
	app.SetAgentReporter(hr.Report, hr.Release, hr.NameTab, func(userID string) (string, bool, bool) {
		// Straight to the DB: detection needs IsBot, which the
		// in-memory name map doesn't carry.
		u, err := db.GetUser(userID)
		if err != nil {
			return "", false, false
		}
		return u.BestName(), u.IsBot, true
	})
	// A crash skips this, leaving a stale sidebar entry until herdr's
	// own pane detection reclaims the pane; only clean exits release.
	return func() { hr.Close(time.Second) }
}

func (h *rtmEventHandler) OnAssistantStatus(channelID, threadTS, botUserID, status string) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.AssistantStatusMsg{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		BotUserID: botUserID,
		Status:    status,
	})
}
