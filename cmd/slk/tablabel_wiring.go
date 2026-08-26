package main

import (
	"context"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/tablabel"
	"github.com/gammons/slk/internal/ui"
)

// labelTimeout bounds one tab-label request. Nothing blocks on it — the
// deterministic label is already on the tab — so it only exists to reap
// the goroutine when the API hangs.
const labelTimeout = 20 * time.Second

// wireAgentTabLabeler installs model-generated tab labels when configured
// (herdr.tab_name_model) and credentialed (ANTHROPIC_API_KEY). Results
// re-enter the program loop as ui.AgentTabLabelMsg; failures are logged
// and dropped, leaving the deterministic label standing.
func wireAgentTabLabeler(app *ui.App, cfg config.Herdr, send func(tea.Msg)) {
	if cfg.TabNameModel == "" {
		return
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		debuglog.Notify("tablabel: tab_name_model set but ANTHROPIC_API_KEY is missing; deterministic labels only")
		return
	}
	gen := tablabel.New(cfg.TabNameModel)
	request := func(teamID, channelID, threadTS string, call func(context.Context) (string, error)) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), labelTimeout)
			defer cancel()
			label, err := call(ctx)
			if err != nil {
				debuglog.Notify("tablabel: %v", err)
				return
			}
			send(ui.AgentTabLabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, Label: label})
		}()
	}
	app.SetAgentTabLabeler(func(teamID, channelID, threadTS, root string) {
		request(teamID, channelID, threadTS, func(ctx context.Context) (string, error) {
			return gen.Label(ctx, root)
		})
	})
	app.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript string) {
		request(teamID, channelID, threadTS, func(ctx context.Context) (string, error) {
			return gen.Relabel(ctx, transcript)
		})
	})
}
