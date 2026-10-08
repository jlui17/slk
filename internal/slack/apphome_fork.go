package slackclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
)

// The calls in this file are the web client's own for an app's Home
// tab: conversations.info with return_app_home reads it,
// apps.home.dispatchOpenEvent tells the app the tab was opened (Slack's
// app_home_opened, which some apps answer by publishing their Home),
// and blocks.actions is a click on one of its elements. The app's
// redraw arrives on the websocket as view_updated (see ViewEvent). The
// request shapes were captured from the web client and replayed from
// slk.

// AppHome is the Home tab of an app DM.
type AppHome struct {
	AppID string
	// TeamID is the team the app is installed in.
	TeamID             string
	HomeTabEnabled     bool
	MessagesTabEnabled bool
	// HomeView is the view object the app published, as Slack sent
	// it; nil when the app has published none.
	HomeView json.RawMessage
}

// GetAppHome reads the Home tab of the app DM channelID.
func (c *Client) GetAppHome(ctx context.Context, channelID string) (AppHome, error) {
	raw, err := c.postForm(ctx, "conversations.info", url.Values{
		"channel":         {channelID},
		"return_app_home": {"true"},
	})
	if err != nil {
		return AppHome{}, err
	}
	if err := parseOKResponse("conversations.info", raw); err != nil {
		return AppHome{}, err
	}
	var resp struct {
		AppHome struct {
			AppID              string `json:"app_id"`
			TeamID             string `json:"app_installed_team_id"`
			HomeTabEnabled     bool   `json:"home_tab_enabled"`
			MessagesTabEnabled bool   `json:"messages_tab_enabled"`
		} `json:"app_home"`
		HomeView json.RawMessage `json:"home_view"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return AppHome{}, fmt.Errorf("parsing conversations.info: %w", err)
	}
	home := AppHome{
		AppID:              resp.AppHome.AppID,
		TeamID:             resp.AppHome.TeamID,
		HomeTabEnabled:     resp.AppHome.HomeTabEnabled,
		MessagesTabEnabled: resp.AppHome.MessagesTabEnabled,
	}
	if len(resp.HomeView) > 0 && !bytes.Equal(resp.HomeView, []byte("null")) {
		home.HomeView = resp.HomeView
	}
	return home, nil
}

// DispatchAppHomeOpened tells the app of DM channelID, installed in
// teamID, that its Home tab was opened, as Slack does on every open.
func (c *Client) DispatchAppHomeOpened(ctx context.Context, channelID, teamID string) error {
	raw, err := c.postForm(ctx, "apps.home.dispatchOpenEvent", url.Values{
		"id":              {channelID},
		"type":            {"home"},
		"service_team_id": {teamID},
	})
	if err != nil {
		return err
	}
	return parseOKResponse("apps.home.dispatchOpenEvent", raw)
}

// BlockAction is one click on an interactive element of a view.
type BlockAction struct {
	ViewID string
	// BotID, AppID and TeamID are the view's bot_id, app_id and the
	// team the app is installed in.
	BotID, AppID, TeamID string
	// Action is the element as the view has it, with its block's
	// block_id added, and selected_option for a choice in a select.
	Action json.RawMessage
	// State is the view's input state, {"values": ...}.
	State       json.RawMessage
	ClientToken string
}

// SendBlockAction sends a click. Slack answers only ok; the app's
// redraw, if any, arrives later as view_updated. A refusal is a
// slack.SlackErrorResponse holding Slack's error code, "refused" when
// it sends none.
func (c *Client) SendBlockAction(ctx context.Context, a BlockAction) error {
	raw, err := c.postForm(ctx, "blocks.actions", blockActionForm(a))
	if err != nil {
		return err
	}
	return refusalOf("blocks.actions", raw)
}

// blockActionForm is blocks.actions' form, in the web client's shape.
func blockActionForm(a BlockAction) url.Values {
	container, _ := json.Marshal(map[string]string{"type": "view", "view_id": a.ViewID})
	state := a.State
	if len(state) == 0 {
		state = json.RawMessage(`{"values":{}}`)
	}
	return url.Values{
		"service_id":      {a.BotID},
		"app_id":          {a.AppID},
		"service_team_id": {a.TeamID},
		"actions":         {"[" + string(a.Action) + "]"},
		"container":       {string(container)},
		"client_token":    {a.ClientToken},
		"state":           {string(state)},
	}
}
