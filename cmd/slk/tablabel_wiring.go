package main

import (
	"context"
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

// wireAgentTabLabeler installs the model-backed assists — tab labels (at
// thread open and on :retitle) and the working judge — when configured
// (herdr.tab_name_model) and credentialed (Herdr.ResolveAnthropicAPIKey). Results
// re-enter the program loop as ui messages; failures are logged and leave
// the deterministic behavior standing.
func wireAgentTabLabeler(app *ui.App, cfg config.Herdr, send func(tea.Msg)) {
	if cfg.TabNameModel == "" {
		return
	}
	apiKey := cfg.ResolveAnthropicAPIKey()
	if apiKey == "" {
		debuglog.Notify("tablabel: tab_name_model set but no API key (herdr.anthropic_api_key or ANTHROPIC_API_KEY); deterministic labels only")
		return
	}
	gen := tablabel.New(cfg.TabNameModel, apiKey)
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
	app.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript, fallbackTaskID string) {
		request(func(ctx context.Context) (tea.Msg, error) {
			id, label, err := gen.Relabel(ctx, transcript, cfg.TabNameHints)
			return ui.AgentTabRelabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, TaskID: id, FallbackTaskID: fallbackTaskID, Label: label}, err
		})
	})
	app.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, fromAgent bool) {
		request(func(ctx context.Context) (tea.Msg, error) {
			j, err := gen.Judge(ctx, message, fromAgent)
			msg := ui.AgentWorkingVerdictMsg{
				TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, Key: key, State: ui.AgentState(j.Verdict),
				Reply: j.Reply, Model: cfg.TabNameModel, PromptHash: j.PromptHash, TextHash: j.TextHash,
			}
			if err != nil {
				// Unlike a label, a failed judgment still goes to the ui,
				// which logs it as a judge_error agent-state report.
				debuglog.Notify("tablabel: %v", err)
				msg.Err = err.Error()
			}
			return msg, nil
		})
	})
}
