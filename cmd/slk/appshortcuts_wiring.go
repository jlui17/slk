package main

import (
	"context"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui"
	"github.com/gammons/slk/internal/ui/appshortcuts"
)

func (h *rtmEventHandler) OnView(evt slackclient.ViewEvent) {
	if h.program == nil {
		return
	}
	h.program.Send(ui.AppViewMsg{
		TeamID:      h.workspaceID,
		ClientToken: evt.ClientToken,
		Updated:     evt.Updated,
		View:        evt.View,
	})
}

// appShortcutService is the `.` overlay's appshortcuts.Service, routed
// to the workspace the overlay was opened in.
type appShortcutService struct{ router *workspaceRouter }

func (s appShortcutService) List(ctx context.Context, teamID string) ([]appshortcuts.Shortcut, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return nil, err
	}
	actions, err := c.ListAppActions(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]appshortcuts.Shortcut, len(actions))
	for i, a := range actions {
		out[i] = appshortcuts.Shortcut{AppID: a.AppID, AppName: a.AppName, ActionID: a.ActionID, Name: a.Name}
	}
	return out, nil
}

func (s appShortcutService) Run(ctx context.Context, teamID string, sc appshortcuts.Shortcut, channelID, messageTS, clientToken string) error {
	c, err := s.router.Client(teamID)
	if err != nil {
		return err
	}
	return c.RunAppAction(ctx, sc.AppID, sc.ActionID, channelID, messageTS, clientToken)
}

func (s appShortcutService) Close(ctx context.Context, teamID, viewID, rootViewID, clientToken string) error {
	c, err := s.router.Client(teamID)
	if err != nil {
		return err
	}
	return c.CloseView(ctx, viewID, rootViewID, clientToken)
}

func (s appShortcutService) Submit(ctx context.Context, teamID, viewID, clientToken, state string) (appshortcuts.SubmitResult, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return appshortcuts.SubmitResult{}, err
	}
	res, err := c.SubmitView(ctx, viewID, clientToken, state)
	if err != nil {
		return appshortcuts.SubmitResult{}, err
	}
	return appshortcuts.SubmitResult{Error: res.Error, FieldErrors: res.Errors}, nil
}
