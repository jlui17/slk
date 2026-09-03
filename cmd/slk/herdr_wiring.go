package main

import (
	"os"
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/herdr"
	"github.com/gammons/slk/internal/ui"
)

// wireHerdr connects app's agent-thread reporting and the O
// keybinding's tab opener to the herdr pane slk is running in, when
// there is one. The returned reporter is for wiring that must wait for
// the tea program (the focus watcher's Send); the close func releases
// the pane's sidebar entry and must be deferred by the caller. Both are
// nil when herdr is absent or disabled.
func wireHerdr(app *ui.App, db *cache.DB, cfg config.Herdr) (*herdr.Reporter, func()) {
	hr := herdr.NewReporterFromEnv()
	if hr == nil || cfg.Disabled {
		return nil, nil
	}
	hr.SetPaneIDCache(herdrPaneIDStore(db, os.Getenv("HERDR_PANE_ID")))
	hr.SetTabLabelCache(herdrTabLabelStore(db, os.Getenv("HERDR_PANE_ID")))
	report := func(agent, displayName, title string, state ui.AgentState, statusMessage string) {
		hr.Report(agent, displayName, title, string(state), statusMessage)
	}
	app.SetAgentReporter(report, hr.ReportUnread, hr.NameTab, func(userID string) (string, bool, bool) {
		// Straight to the DB: detection needs IsBot, which the
		// in-memory name map doesn't carry.
		u, err := db.GetUser(userID)
		if err != nil {
			return "", false, false
		}
		return u.BestName(), u.IsBot, true
	})
	if hr.CanOpenTab() {
		openCommand := cfg.OpenCommand
		if openCommand == "" {
			openCommand = "slk"
		}
		app.SetHerdrTabOpener(func(url, label string) error {
			return hr.OpenTab(label, openCommand, url)
		})
	}
	// A crash skips this, leaving a stale sidebar entry until herdr's
	// own pane detection reclaims the pane; only clean exits release.
	return hr, func() { hr.Close(time.Second) }
}

// herdrPaneIDStore returns the reporter's pane-id cache hooks over the
// DB, keyed by the launch env's pane id (non-empty whenever the
// reporter is): what the store remembers is where that id's pane ended
// up.
func herdrPaneIDStore(db *cache.DB, paneKey string) (load func() (string, bool), save func(string) error) {
	load = func() (string, bool) {
		id, ok, err := db.GetHerdrPaneID(paneKey)
		if err != nil {
			// A read error looks identical to a cold cache from the
			// recovery path; the log line is the only way to tell a row
			// existed but could not be read.
			debuglog.Notify("herdr: pane id cache read: %v", err)
			return "", false
		}
		return id, ok
	}
	save = func(paneID string) error {
		return db.RecordHerdrPaneID(paneKey, paneID)
	}
	return load, save
}

// herdrTabLabelStore returns the reporter's tab-label cache hooks over the
// DB, keyed like the pane-id store by the launch env's pane id.
func herdrTabLabelStore(db *cache.DB, paneKey string) (load func() (string, bool), save func(string) error) {
	load = func() (string, bool) {
		label, ok, err := db.GetHerdrTabLabel(paneKey)
		if err != nil {
			debuglog.Notify("herdr: tab label cache read: %v", err)
			return "", false
		}
		return label, ok
	}
	save = func(label string) error {
		return db.RecordHerdrTabLabel(paneKey, label)
	}
	return load, save
}

func (h *rtmEventHandler) OnAssistantStatus(channelID, threadTS, botUserID, status string) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.AssistantStatusMsg{
		TeamID:    h.workspaceID,
		ChannelID: channelID,
		ThreadTS:  threadTS,
		BotUserID: botUserID,
		Status:    status,
	})
}
