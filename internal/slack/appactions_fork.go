package slackclient

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/debuglog"
)

// The calls in this file are the web client's own for an app's message
// shortcut and the modal it opens: apps.actions.list, then
// apps.actions.v2.execute, after which the modal arrives on the
// websocket as view_opened (see ViewEvent), then views.submit or
// views.close. None of them is in Slack's public API; the request
// shapes were captured from the web client and replayed from slk.

// AppAction is one message shortcut an installed app offers.
type AppAction struct {
	AppID    string
	AppName  string
	ActionID string
	Name     string
}

// ListAppActions returns the message shortcuts of every app installed
// in the workspace, in Slack's order. Global shortcuts are left out.
func (c *Client) ListAppActions(ctx context.Context) ([]AppAction, error) {
	raw, err := c.postForm(ctx, "apps.actions.list", nil)
	if err != nil {
		return nil, err
	}
	if err := parseOKResponse("apps.actions.list", raw); err != nil {
		return nil, err
	}
	var resp struct {
		AppActions []struct {
			AppID   string `json:"app_id"`
			AppName string `json:"app_name"`
			Actions []struct {
				ActionID string `json:"action_id"`
				Name     string `json:"name"`
				Type     string `json:"type"`
			} `json:"actions"`
		} `json:"app_actions"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("parsing apps.actions.list: %w", err)
	}
	var out []AppAction
	for _, app := range resp.AppActions {
		for _, a := range app.Actions {
			if a.Type != "message_action" {
				continue
			}
			out = append(out, AppAction{AppID: app.AppID, AppName: app.AppName, ActionID: a.ActionID, Name: a.Name})
		}
	}
	return out, nil
}

// RunAppAction runs a message shortcut on the message at messageTS in
// channelID, a thread reply included. Slack answers only ok; the app's
// modal, if it opens one, arrives later as a view_opened event that
// echoes clientToken. A refusal is a slack.SlackErrorResponse, whose
// text is Slack's bare error code, for the overlay to show; "refused"
// when Slack sends ok false without one.
func (c *Client) RunAppAction(ctx context.Context, appID, actionID, channelID, messageTS, clientToken string) error {
	msgContext, err := json.Marshal(struct {
		ChannelID string `json:"channel_id"`
		MessageTS string `json:"message_ts"`
	}{channelID, messageTS})
	if err != nil {
		return err
	}
	raw, err := c.postForm(ctx, "apps.actions.v2.execute", url.Values{
		"action_id":    {actionID},
		"app_id":       {appID},
		"client_token": {clientToken},
		"context":      {string(msgContext)},
	})
	if err != nil {
		return err
	}
	var resp slack.SlackResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing apps.actions.v2.execute: %w (body=%s)", err, truncateForLog(raw))
	}
	if !resp.Ok {
		return slack.SlackErrorResponse{Err: cmp.Or(resp.Error, "refused")}
	}
	return nil
}

// CloseView dismisses an open modal, as its close button does.
func (c *Client) CloseView(ctx context.Context, viewID, rootViewID, clientToken string) error {
	raw, err := c.postForm(ctx, "views.close", url.Values{
		"view_id":      {viewID},
		"root_view_id": {rootViewID},
		"client_token": {clientToken},
	})
	if err != nil {
		return err
	}
	return parseOKResponse("views.close", raw)
}

// ViewSubmitResult is Slack's answer to views.submit: accepted when it
// holds neither an Error nor Errors. An app that rejects the input
// answers response_action "errors" with a message per block_id; whether
// Slack relays that as ok with response_action set, or as ok false with
// errors, has not been seen live, so both shapes land in Errors.
type ViewSubmitResult struct {
	// Error is Slack's error code for an ok false answer, "refused" when
	// it sends none.
	Error  string
	Errors map[string]string
}

// SubmitView sends a modal's input, state being the JSON of
// {"values":{block_id:{action_id:value}}}. The error is for transport
// and parse failures only; Slack's own answer is in the result.
func (c *Client) SubmitView(ctx context.Context, viewID, clientToken, state string) (ViewSubmitResult, error) {
	raw, err := c.postForm(ctx, "views.submit", url.Values{
		"view_id":      {viewID},
		"client_token": {clientToken},
		"state":        {state},
	})
	if err != nil {
		return ViewSubmitResult{}, err
	}
	var resp struct {
		OK     bool            `json:"ok"`
		Error  string          `json:"error"`
		Errors json.RawMessage `json:"errors"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ViewSubmitResult{}, fmt.Errorf("parsing views.submit: %w (body=%s)", err, truncateForLog(raw))
	}
	var res ViewSubmitResult
	if !resp.OK {
		res.Error = cmp.Or(resp.Error, "refused")
	}
	// Only an object is the app's field errors; Slack's own errors list
	// on an ok false answer is left to the error code.
	if fields := map[string]string{}; json.Unmarshal(resp.Errors, &fields) == nil && len(fields) > 0 {
		res.Errors = fields
	}
	return res, nil
}

// ViewEvent is a view_opened or view_updated websocket event: an app
// opened a modal for this user, or changed the one open.
type ViewEvent struct {
	Updated bool
	// ClientToken is, on view_opened, the token of the RunAppAction
	// that asked for the modal.
	ClientToken string
	// View is the view object as Slack sent it.
	View json.RawMessage
}

func dispatchViewEvent(data []byte, eventType string, handler EventHandler) {
	var evt struct {
		ClientToken string          `json:"client_token"`
		ViewID      string          `json:"view_id"`
		View        json.RawMessage `json:"view"`
	}
	if err := json.Unmarshal(data, &evt); err != nil || len(evt.View) == 0 {
		return
	}
	debuglog.WS("%s: view=%s client_token=%s", eventType, evt.ViewID, evt.ClientToken)
	handler.OnView(ViewEvent{Updated: eventType == "view_updated", ClientToken: evt.ClientToken, View: evt.View})
}
