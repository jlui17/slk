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

// wireAgentTabLabeler installs the model-backed assists — tab labels,
// :retitle, and the working judge — when configured (herdr.tab_name_model)
// and credentialed (ANTHROPIC_API_KEY). Results re-enter the program loop
// as ui messages; failures are logged and dropped, leaving the
// deterministic behavior standing.
func wireAgentTabLabeler(app *ui.App, cfg config.Herdr, send func(tea.Msg)) {
	if cfg.TabNameModel == "" {
		return
	}
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		debuglog.Notify("tablabel: tab_name_model set but ANTHROPIC_API_KEY is missing; deterministic labels only")
		return
	}
	gen := tablabel.New(cfg.TabNameModel)
	request := func(call func(context.Context) (tea.Msg, error)) {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), labelTimeout)
			defer cancel()
			msg, err := call(ctx)
			if err != nil {
				debuglog.Notify("tablabel: %v", err)
				return
			}
			send(msg)
		}()
	}
	app.SetAgentTabLabeler(func(teamID, channelID, threadTS, root string) {
		request(func(ctx context.Context) (tea.Msg, error) {
			label, err := gen.Label(ctx, root)
			return ui.AgentTabLabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, Label: label}, err
		})
	})
	app.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript string) {
		request(func(ctx context.Context) (tea.Msg, error) {
			id, label, err := gen.Relabel(ctx, transcript, cfg.TabNameHints)
			return ui.AgentTabRelabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, TaskID: id, Label: label}, err
		})
	})
	app.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, fromAgent bool) {
		request(func(ctx context.Context) (tea.Msg, error) {
			verdict, err := gen.Judge(ctx, message, fromAgent)
			return ui.AgentWorkingVerdictMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, Key: key, State: ui.AgentState(verdict)}, err
		})
	})
}
