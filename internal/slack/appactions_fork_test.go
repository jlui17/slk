package slackclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"testing"

	"github.com/slack-go/slack"
)

func (m *mockEventHandler) OnView(ViewEvent) {}

// viewRecorder is a mockEventHandler that keeps the views it is handed.
type viewRecorder struct {
	mockEventHandler
	views []ViewEvent
}

func (r *viewRecorder) OnView(evt ViewEvent) { r.views = append(r.views, evt) }

// appActionsServer answers every call with body and records the
// method and form of the last request.
func appActionsServer(t *testing.T, body string) (*Client, *string, *url.Values) {
	t.Helper()
	var method string
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.URL.Path
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		form = r.PostForm
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return &Client{token: "xoxc-test", apiBaseURL: srv.URL + "/api/"}, &method, &form
}

func TestListAppActions_CapturedResponse(t *testing.T) {
	body, err := os.ReadFile("testdata/apps_actions_list.json")
	if err != nil {
		t.Fatal(err)
	}
	c, method, _ := appActionsServer(t, string(body))
	got, err := c.ListAppActions(context.Background())
	if err != nil {
		t.Fatalf("ListAppActions: %v", err)
	}
	if *method != "/api/apps.actions.list" {
		t.Errorf("method = %s", *method)
	}
	want := []AppAction{{AppID: "A00000APP01", AppName: "Colony", ActionID: "10000000000001", Name: "Annotate"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestListAppActions_SkipsGlobalShortcuts(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":true,"app_actions":[{"app_id":"A1","app_name":"X","actions":[
		{"action_id":"1","name":"New thing","type":"global_action"},
		{"action_id":"2","name":"Tag","type":"message_action"}]}]}`)
	got, err := c.ListAppActions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "Tag" {
		t.Errorf("got %+v, want only the message action", got)
	}
}

func TestRunAppAction_Form(t *testing.T) {
	c, method, form := appActionsServer(t, `{"ok":true}`)
	if err := c.RunAppAction(context.Background(), "A1", "123", "C1", "1700000000.000100", "web-1700000000000"); err != nil {
		t.Fatal(err)
	}
	if *method != "/api/apps.actions.v2.execute" {
		t.Errorf("method = %s", *method)
	}
	want := map[string]string{
		"token":        "xoxc-test",
		"app_id":       "A1",
		"action_id":    "123",
		"client_token": "web-1700000000000",
		"context":      `{"channel_id":"C1","message_ts":"1700000000.000100"}`,
	}
	for k, v := range want {
		if got := form.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
}

func TestRunAppAction_NotOK(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":false,"error":"not_in_channel"}`)
	err := c.RunAppAction(context.Background(), "A1", "123", "C1", "1.2", "web-1")
	var se slack.SlackErrorResponse
	if !errors.As(err, &se) || err.Error() != "not_in_channel" {
		t.Fatalf("err = %v, want the bare SlackErrorResponse not_in_channel", err)
	}
}

// ok false without an error code is still a refusal, never success.
func TestRunAppAction_NotOKWithoutCode(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":false}`)
	err := c.RunAppAction(context.Background(), "A1", "123", "C1", "1.2", "web-1")
	if err == nil || err.Error() != "refused" {
		t.Fatalf("err = %v, want refused", err)
	}
}

// Every ok false answer to views.submit is a refusal with an error code,
// "refused" when Slack sends none; errors is read as the app's field
// errors only when it is an object.
func TestSubmitView_Refusals(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
		wantFields          map[string]string
	}{
		{"no code", `{"ok":false}`, "refused", nil},
		{"errors not an object", `{"ok":false,"error":"invalid_arguments","errors":["bad state"]}`, "invalid_arguments", nil},
		{"field errors", `{"ok":false,"error":"validation_failed","errors":{"note":"Too short"}}`, "validation_failed", map[string]string{"note": "Too short"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := appActionsServer(t, tc.body)
			got, err := c.SubmitView(context.Background(), "V1", "web-3", `{"values":{}}`)
			if err != nil {
				t.Fatal(err)
			}
			if got.Error != tc.wantErr || !reflect.DeepEqual(got.Errors, tc.wantFields) {
				t.Errorf("error %q fields %v, want %q %v", got.Error, got.Errors, tc.wantErr, tc.wantFields)
			}
		})
	}
}

func TestCloseView_Form(t *testing.T) {
	c, method, form := appActionsServer(t, `{"ok":true}`)
	if err := c.CloseView(context.Background(), "V1", "V0", "web-2"); err != nil {
		t.Fatal(err)
	}
	if *method != "/api/views.close" || form.Get("view_id") != "V1" || form.Get("root_view_id") != "V0" || form.Get("client_token") != "web-2" {
		t.Errorf("method %s form %v", *method, *form)
	}
}

func TestSubmitView(t *testing.T) {
	cases := []struct {
		name string
		body string
		want ViewSubmitResult
	}{
		{"ok", `{"ok":true,"view":null,"response_action":null}`, ViewSubmitResult{}},
		{"errors action", `{"ok":true,"response_action":"errors","errors":{"note":"Too short"}}`,
			ViewSubmitResult{Errors: map[string]string{"note": "Too short"}}},
		{"validation failed", `{"ok":false,"error":"validation_failed","errors":{"note":"Too short"}}`,
			ViewSubmitResult{Error: "validation_failed", Errors: map[string]string{"note": "Too short"}}},
		{"not ok", `{"ok":false,"error":"view_not_found"}`, ViewSubmitResult{Error: "view_not_found"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, method, form := appActionsServer(t, tc.body)
			got, err := c.SubmitView(context.Background(), "V1", "web-3", `{"values":{}}`)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if *method != "/api/views.submit" || form.Get("view_id") != "V1" || form.Get("client_token") != "web-3" || form.Get("state") != `{"values":{}}` {
				t.Errorf("method %s form %v", *method, *form)
			}
		})
	}
}

func TestDispatchViewOpened_CapturedEvent(t *testing.T) {
	data, err := os.ReadFile("testdata/ws_view_opened.json")
	if err != nil {
		t.Fatal(err)
	}
	handler := &viewRecorder{}
	dispatchWebSocketEvent(data, handler)
	if len(handler.views) != 1 {
		t.Fatalf("got %d views, want 1", len(handler.views))
	}
	evt := handler.views[0]
	if evt.Updated || evt.ClientToken != "web-1700000000000" {
		t.Errorf("got %+v", evt)
	}
	var view struct {
		ID   string `json:"id"`
		Hash string `json:"hash"`
	}
	if err := json.Unmarshal(evt.View, &view); err != nil || view.ID != "V00000VIEW1" {
		t.Errorf("view = %+v, err %v; want the nested view object", view, err)
	}
}

func TestDispatchViewUpdated(t *testing.T) {
	handler := &viewRecorder{}
	dispatchWebSocketEvent([]byte(`{"type":"view_updated","view_id":"V1","view":{"id":"V1","type":"modal"}}`), handler)
	if len(handler.views) != 1 || !handler.views[0].Updated {
		t.Fatalf("got %+v, want one updated view", handler.views)
	}
}
