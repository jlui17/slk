package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/slack-go/slack"
)

func TestGetAppHome_CapturedResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/conversations_info_app_home.json")
	if err != nil {
		t.Fatal(err)
	}
	c, method, form := appActionsServer(t, string(body))
	got, err := c.GetAppHome(context.Background(), "D00000COLONY")
	if err != nil {
		t.Fatalf("GetAppHome: %v", err)
	}
	if *method != "/api/conversations.info" {
		t.Errorf("method = %s", *method)
	}
	if form.Get("channel") != "D00000COLONY" || form.Get("return_app_home") != "true" {
		t.Errorf("form = %v", *form)
	}
	if got.AppID != "A00000APP01" || got.TeamID != "T00000TEAM1" || !got.HomeTabEnabled || !got.MessagesTabEnabled {
		t.Errorf("got %+v", got)
	}
	var view struct {
		ID     string            `json:"id"`
		BotID  string            `json:"bot_id"`
		Blocks []json.RawMessage `json:"blocks"`
	}
	if err := json.Unmarshal(got.HomeView, &view); err != nil {
		t.Fatal(err)
	}
	if view.ID != "V00000HOME1" || view.BotID != "B00000BOT01" || len(view.Blocks) != 13 {
		t.Errorf("home view = %s, %s, %d blocks", view.ID, view.BotID, len(view.Blocks))
	}
}

// The Claude app's Home before it has published one: enabled, with a
// null home_view.
func TestGetAppHome_NoViewPublished(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":true,"app_home":{"app_id":"A2","home_tab_enabled":true,"messages_tab_enabled":true,"home_view_id":null},"home_view":null,"channel":{"id":"D2"}}`)
	got, err := c.GetAppHome(context.Background(), "D2")
	if err != nil {
		t.Fatalf("GetAppHome: %v", err)
	}
	if !got.HomeTabEnabled || got.HomeView != nil {
		t.Errorf("got %+v, want home enabled with no view", got)
	}
}

func TestDispatchAppHomeOpened_Form(t *testing.T) {
	c, method, form := appActionsServer(t, `{"ok":true}`)
	if err := c.DispatchAppHomeOpened(context.Background(), "D1", "T1"); err != nil {
		t.Fatal(err)
	}
	want := url.Values{"token": {"xoxc-test"}, "id": {"D1"}, "type": {"home"}, "service_team_id": {"T1"}}
	if *method != "/api/apps.home.dispatchOpenEvent" || !reflect.DeepEqual(*form, want) {
		t.Errorf("%s %v, want %v", *method, *form, want)
	}
}

// The form fields as the web client sends them for a click on a Home
// tab's select.
func TestSendBlockAction_Form(t *testing.T) {
	c, method, form := appActionsServer(t, `{"ok":true}`)
	err := c.SendBlockAction(context.Background(), BlockAction{
		ViewID:      "V1",
		BotID:       "B1",
		AppID:       "A1",
		TeamID:      "T1",
		Action:      json.RawMessage(`{"type":"static_select","action_id":"sort_order","block_id":"b1","selected_option":{"text":{"type":"plain_text","text":"Oldest first"},"value":"oldest"}}`),
		State:       json.RawMessage(`{"values":{"b1":{"sort_order":{"type":"static_select","selected_option":{"value":"oldest"}}}}}`),
		ClientToken: "web-1700000000000",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := url.Values{
		"token":           {"xoxc-test"},
		"service_id":      {"B1"},
		"app_id":          {"A1"},
		"service_team_id": {"T1"},
		"actions":         {`[{"type":"static_select","action_id":"sort_order","block_id":"b1","selected_option":{"text":{"type":"plain_text","text":"Oldest first"},"value":"oldest"}}]`},
		"container":       {`{"type":"view","view_id":"V1"}`},
		"client_token":    {"web-1700000000000"},
		"state":           {`{"values":{"b1":{"sort_order":{"type":"static_select","selected_option":{"value":"oldest"}}}}}`},
	}
	if *method != "/api/blocks.actions" {
		t.Errorf("method = %s", *method)
	}
	if !reflect.DeepEqual(*form, want) {
		t.Errorf("form =\n%v\nwant\n%v", *form, want)
	}
}

func TestSendBlockAction_Refused(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":false}`)
	err := c.SendBlockAction(context.Background(), BlockAction{Action: json.RawMessage(`{}`)})
	var refused slack.SlackErrorResponse
	if !errors.As(err, &refused) || refused.Err != "refused" {
		t.Errorf("err = %v, want refused", err)
	}
}
