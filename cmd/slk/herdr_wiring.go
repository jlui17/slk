package main

import (
	"os"
	"strings"
	"time"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/herdr"
	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui"
)

// wireHerdr connects app's agent-thread reporting, the O keybinding's
// tab opener, alt+o's pane opener and the pane's resume command to the
// herdr pane slk is running in, when there is one; tokens name the
// workspaces whose threads the resume command links to. The returned
// client is for wiring that must wait for the tea program (the focus
// watcher's Send); the close func releases the pane's sidebar entry and
// resume command and must be deferred by the caller. Both are nil when
// herdr is absent or disabled.
func wireHerdr(app *ui.App, db *cache.DB, cfg config.Herdr, tokens []slackclient.Token) (*herdr.Client, func()) {
	hr := herdr.NewClientFromEnv()
	if hr == nil || cfg.Disabled {
		return nil, nil
	}
	paneKey := os.Getenv("HERDR_PANE_ID")
	hr.SetPaneIDCache(herdrPaneCache(paneKey, "pane id", db.GetHerdrPaneID, db.RecordHerdrPaneID))
	hr.SetTabLabelCache(herdrPaneCache(paneKey, "tab label", db.GetHerdrTabLabel, db.RecordHerdrTabLabel))
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
	app.SetAgentThreadMarks(db)
	app.SetAgentTabForceNamer(hr.ForceNameTab)
	openCommand := cfg.ResolveOpenCommand()
	if hr.CanOpenTab() {
		app.SetHerdrTabOpener(func(url, label string, focus bool) error {
			return hr.OpenTab(label, openCommand, url, focus)
		})
	}
	// A split needs only slk's own pane id, which every client has.
	app.SetHerdrPaneOpener(func(url, splitPaneID string, focus bool) (string, error) {
		return hr.OpenPane(splitPaneID, openCommand, url, focus)
	})
	domains := map[string]string{}
	for _, t := range tokens {
		domains[t.TeamID] = t.Domain
	}
	launcher := resumeLauncher(openCommand, os.Getenv)
	app.SetPaneResumeReporter(func(teamID, channelID, threadTS, title string) {
		link := slackurl.ThreadPermalink(domains[teamID], channelID, threadTS)
		hr.ReportResume(herdr.ResumeArgv(launcher, link), title)
	})
	// A crash skips this, leaving a stale sidebar entry until herdr's
	// own pane detection reclaims the pane; only clean exits release.
	return hr, func() { hr.Close(time.Second) }
}

// herdrPaneCache returns one of the client's per-pane cache hooks
// (pane id, tab label) over the DB row get and put read and write,
// keyed by the launch env's pane id (non-empty whenever the client
// is): what the row remembers is about that id's pane wherever it
// ended up. what names the value in the read-error log line.
func herdrPaneCache(paneKey, what string, get func(paneKey string) (string, bool, error), put func(paneKey, value string) error) (load func() (string, bool), save func(string) error) {
	load = func() (string, bool) {
		v, ok, err := get(paneKey)
		if err != nil {
			// A read error looks identical to a cold cache from the
			// recovery path; the log line is the only way to tell a row
			// existed but could not be read.
			debuglog.Notify("herdr: %s cache read: %v", what, err)
			return "", false
		}
		return v, ok
	}
	save = func(v string) error {
		return put(paneKey, v)
	}
	return load, save
}

// resumeLauncher is the host command that starts this slk again, for the
// resume command herdr runs after a restart. tools/run-docker.sh names
// itself and its role (SLK_HOST_LAUNCHER, SLK_ROLE), so a pane comes back
// on the checkout and role it ran; slk run any other way comes back
// through openCommand, the launcher O uses.
func resumeLauncher(openCommand string, getenv func(string) string) []string {
	if launcher := getenv("SLK_HOST_LAUNCHER"); launcher != "" {
		return []string{"env", "SLK_ROLE=" + getenv("SLK_ROLE"), launcher}
	}
	return strings.Fields(openCommand)
}
