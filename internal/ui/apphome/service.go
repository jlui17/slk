package apphome

import (
	"context"
	"encoding/json"
)

// Home is an app DM's Home tab as conversations.info returns it.
type Home struct {
	AppID string
	// TeamID is the team the app is installed in.
	TeamID  string
	Enabled bool
	// MessagesTab is false for an app that has only its Home tab.
	MessagesTab bool
	// View is the published view object, nil for none.
	View []byte
}

// Click is one click on a Home view's element.
type Click struct {
	ViewID, BotID, AppID, TeamID string
	Action, State                json.RawMessage
	ClientToken                  string
}

// Service reads and acts on an app's Home tab in workspace teamID.
type Service interface {
	Load(ctx context.Context, teamID, channelID string) (Home, error)
	// Opened tells the app its Home tab was opened.
	Opened(ctx context.Context, teamID, channelID, appTeamID string) error
	Click(ctx context.Context, teamID string, c Click) error
}
