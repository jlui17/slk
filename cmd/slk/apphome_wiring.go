package main

import (
	"context"

	slackclient "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui/apphome"
)

// appHomeService is App Home's apphome.Service, routed to the workspace
// the app DM is in.
type appHomeService struct{ router *workspaceRouter }

func (s appHomeService) Load(ctx context.Context, teamID, channelID string) (apphome.Home, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return apphome.Home{}, err
	}
	h, err := c.GetAppHome(ctx, channelID)
	if err != nil {
		return apphome.Home{}, err
	}
	return apphome.Home{AppID: h.AppID, TeamID: h.TeamID, Enabled: h.HomeTabEnabled, MessagesTab: h.MessagesTabEnabled, View: h.HomeView}, nil
}

func (s appHomeService) Opened(ctx context.Context, teamID, channelID, appTeamID string) error {
	c, err := s.router.Client(teamID)
	if err != nil {
		return err
	}
	return c.DispatchAppHomeOpened(ctx, channelID, appTeamID)
}

func (s appHomeService) Click(ctx context.Context, teamID string, click apphome.Click) error {
	c, err := s.router.Client(teamID)
	if err != nil {
		return err
	}
	return c.SendBlockAction(ctx, slackclient.BlockAction{
		ViewID:      click.ViewID,
		BotID:       click.BotID,
		AppID:       click.AppID,
		TeamID:      click.TeamID,
		Action:      click.Action,
		State:       click.State,
		ClientToken: click.ClientToken,
	})
}
