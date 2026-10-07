package main

import (
	"context"
	"slices"
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
// re-enter the program loop as ui messages; a failed label request is
// logged and dropped, leaving the deterministic label standing, and a
// failed judge request is logged and re-enters as a failed verdict.
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
	app.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript, fallbackTaskID string, force, reviewOpen bool) {
		request(func(ctx context.Context) (tea.Msg, error) {
			id, label, err := gen.Relabel(ctx, transcript, relabelHints(cfg.TabNameHints, reviewOpen))
			return ui.AgentTabRelabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, TaskID: id, FallbackTaskID: fallbackTaskID, Force: force, ReviewOpen: reviewOpen, Label: label}, err
		})
	})
	app.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, earlier []string, fromAgent bool) {
		request(func(ctx context.Context) (tea.Msg, error) {
			start := time.Now()
			verdict, err := gen.Judge(ctx, message, earlier, fromAgent)
			// key is the message ts, the author's side and a hash of the
			// text; the text itself is never logged.
			debuglog.Notify("tablabel: working judge key=%s from_agent=%t text_len=%d earlier=%d verdict=%s err=%v duration=%s",
				key, fromAgent, len(message), len(earlier), verdict, err, time.Since(start).Round(time.Millisecond))
			// A failure re-enters the loop too: the message reads working
			// while the request is in flight, and only the loop can end that.
			return ui.AgentWorkingVerdictMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, Key: key, State: ui.AgentState(verdict), Failed: err != nil}, nil
		})
	})
}

// relabelHints is the user's tab_name_hints, followed by
// tablabel.ReviewOpenHints for a !review-open thread's request. The
// review-open list is a fresh slice: requests run concurrently and must not
// append into the config's backing array.
func relabelHints(userHints []string, reviewOpen bool) []string {
	if !reviewOpen {
		return userHints
	}
	return slices.Concat(userHints, tablabel.ReviewOpenHints)
}
