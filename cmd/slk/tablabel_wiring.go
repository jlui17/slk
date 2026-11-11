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

// credentialTimeout bounds the first ant login token fetch at startup,
// which holds up the TUI's start. The host bridge fetches the token as it
// starts, ahead of slk, so it usually has one ready.
const credentialTimeout = 5 * time.Second

// wireAgentTabLabeler installs the model-backed assists — tab labels (at
// thread open and on :retitle) and the working judge, each with its own
// model and effort — when configured (herdr.tab_name_model) and
// credentialed (tablabel.ResolveCredential). Results
// re-enter the program loop as ui messages; a failed label request is
// logged and dropped, leaving the deterministic label standing, and a
// failed judge request is logged and re-enters as a failed verdict.
func wireAgentTabLabeler(app *ui.App, cfg config.Herdr, send func(tea.Msg)) {
	if cfg.TabNameModel == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), credentialTimeout)
	cred, err := tablabel.ResolveCredential(ctx, cfg, antLoginTokens())
	cancel()
	if err != nil {
		debuglog.Notify("tablabel: tab_name_model set but %v; deterministic labels only", err)
		return
	}
	debuglog.Notify("tablabel: model calls authenticate with %s", cred.Source)
	labeler := tablabel.New(cfg.TabNameModel, cfg.ResolveTabNameEffort(), cred)
	judge := tablabel.New(cfg.ResolveAgentStatusJudgeModel(), cfg.ResolveAgentStatusJudgeEffort(), cred)
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
	app.SetAgentTabRelabeler(func(teamID, channelID, threadTS, transcript, fallbackTaskID string, retitleGen uint64, reviewOpen bool) {
		request(func(ctx context.Context) (tea.Msg, error) {
			id, label, err := labeler.Relabel(ctx, transcript, relabelHints(cfg.TabNameHints, reviewOpen))
			return ui.AgentTabRelabelMsg{TeamID: teamID, ChannelID: channelID, ThreadTS: threadTS, TaskID: id, FallbackTaskID: fallbackTaskID, RetitleGen: retitleGen, ReviewOpen: reviewOpen, Label: label}, err
		})
	})
	app.SetAgentWorkingJudge(func(teamID, channelID, threadTS, key, message string, earlier []string, author tablabel.Author, usersClaudeCodePost string) {
		request(func(ctx context.Context) (tea.Msg, error) {
			start := time.Now()
			verdict, err := judge.Judge(ctx, message, earlier, author, usersClaudeCodePost)
			// key is the message ts, the author's side and a hash of the
			// text; the text itself is never logged.
			debuglog.Notify("tablabel: working judge key=%s author=%q text_len=%d earlier=%d users_claude_code_post_len=%d verdict=%s err=%v duration=%s",
				key, author, len(message), len(earlier), len(usersClaudeCodePost), verdict, err, time.Since(start).Round(time.Millisecond))
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
