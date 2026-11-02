package slackclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/slack-go/slack"
)

// A pins.list answer in the shape Slack documents: a message pin, a
// thread reply pinned later, a bot's message pinned last, and a file.
const pinsListBody = `{"ok":true,"items":[
 {"type":"message","created":1700000100,"created_by":"U9","channel":"C1","message":{"type":"message","user":"U1","text":"Rollback steps","ts":"1699990000.000100","permalink":"https://acme.slack.com/archives/C1/p1699990000000100"}},
 {"type":"message","created":1700000300,"created_by":"U9","channel":"C1","message":{"type":"message","user":"U2","text":"Root cause","ts":"1699995000.000200","thread_ts":"1699994000.000100","permalink":"https://acme.slack.com/archives/C1/p1699995000000200?thread_ts=1699994000.000100&cid=C1"}},
 {"type":"message","created":1700000400,"created_by":"U9","channel":"C1","message":{"type":"message","bot_id":"B1","bot_profile":{"name":"deploybot"},"text":"","ts":"1699996000.000300","files":[{"name":"report.pdf"}]}},
 {"type":"file","created":1700000200,"created_by":"U9","file":{"id":"F1","name":"handbook.pdf","permalink":"https://acme.slack.com/files/U1/F1/handbook.pdf"}}
]}`

func TestListPins_NewestPinnedFirst(t *testing.T) {
	c, method, form := appActionsServer(t, pinsListBody)
	got, err := c.ListPins(context.Background(), "C1")
	if err != nil {
		t.Fatalf("ListPins: %v", err)
	}
	if *method != "/api/pins.list" || form.Get("channel") != "C1" {
		t.Errorf("%s %v", *method, *form)
	}
	want := []Pin{
		{Created: 1700000400, TS: "1699996000.000300", Username: "deploybot", FileName: "report.pdf"},
		{Created: 1700000300, TS: "1699995000.000200", ThreadTS: "1699994000.000100", UserID: "U2", Text: "Root cause",
			Permalink: "https://acme.slack.com/archives/C1/p1699995000000200?thread_ts=1699994000.000100&cid=C1"},
		{Created: 1700000200, FileName: "handbook.pdf", Permalink: "https://acme.slack.com/files/U1/F1/handbook.pdf"},
		{Created: 1700000100, TS: "1699990000.000100", UserID: "U1", Text: "Rollback steps",
			Permalink: "https://acme.slack.com/archives/C1/p1699990000000100"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestListBookmarks_RankOrderAndFolders(t *testing.T) {
	c, method, form := appActionsServer(t, `{"ok":true,"bookmarks":[
	 {"id":"Bk3","title":"Status page","link":"https://status.acme.com","type":"link","rank":"c"},
	 {"id":"Bk1","title":"Runbook","link":"https://wiki.acme.com/runbook","type":"link","rank":"a"},
	 {"id":"Bk2","title":"Dashboards","type":"folder","rank":"b"},
	 {"id":"Bk5","title":"Errors","link":"https://grafana.acme.com/errors","type":"link","rank":"b","parent_id":"Bk2"},
	 {"id":"Bk4","title":"Latency","link":"https://grafana.acme.com/latency","type":"link","rank":"a","parent_id":"Bk2"},
	 {"id":"Bk6","title":null,"link":"","type":"pinned_message","rank":"U","parent_id":null}
	]}`)
	got, err := c.ListBookmarks(context.Background(), "C1")
	if err != nil {
		t.Fatalf("ListBookmarks: %v", err)
	}
	if *method != "/api/bookmarks.list" || form.Get("channel_id") != "C1" {
		t.Errorf("%s %v", *method, *form)
	}
	want := []Bookmark{
		{Title: "Runbook", Link: "https://wiki.acme.com/runbook"},
		{Title: "Latency", Link: "https://grafana.acme.com/latency", Folder: "Dashboards"},
		{Title: "Errors", Link: "https://grafana.acme.com/errors", Folder: "Dashboards"},
		{Title: "Status page", Link: "https://status.acme.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got  %+v\nwant %+v", got, want)
	}
}

func TestPinsBookmarks_Refusals(t *testing.T) {
	var rl *slack.RateLimitedError
	c, _, _ := appActionsServer(t, `{"ok":false,"error":"ratelimited"}`)
	if _, err := c.ListPins(context.Background(), "C1"); !errors.As(err, &rl) {
		t.Errorf("ratelimited answer: err = %v, want a *slack.RateLimitedError", err)
	}
	c, _, _ = appActionsServer(t, `{"ok":false,"error":"channel_not_found"}`)
	_, err := c.ListBookmarks(context.Background(), "C1")
	if err == nil || errors.As(err, &rl) || err.Error() != "bookmarks.list: channel_not_found" {
		t.Errorf("refusal: err = %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	c = &Client{token: "xoxc-test", apiBaseURL: srv.URL + "/api/"}
	if _, err := c.ListPins(context.Background(), "C1"); !errors.As(err, &rl) || rl.RetryAfter != 3*time.Second {
		t.Errorf("429: err = %v, want a *slack.RateLimitedError retrying after 3s", err)
	}
}

// GetHistory, the history read on opening a channel, passes on the
// pin_count Slack answers with.
func TestGetHistory_NotifiesPinCount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"messages":[],"has_more":false,"pin_count":2}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{api: slack.New("xoxc-test", slack.OptionAPIURL(srv.URL+"/"))}
	var got []string
	c.SetPinCountNotify(func(channelID string, count int, _ time.Time) {
		got = append(got, fmt.Sprintf("%s=%d", channelID, count))
	})
	if _, err := c.GetHistory(context.Background(), "C1", 50, ""); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"C1=2"}) {
		t.Errorf("pin counts %v, want [C1=2]", got)
	}
}

type pinsRecorder struct {
	mockEventHandler
	events []string
}

func (r *pinsRecorder) OnPinsChanged(channelID string, added bool) {
	r.events = append(r.events, fmt.Sprintf("%s %v", channelID, added))
}

func TestDispatchPinEvents(t *testing.T) {
	h := &pinsRecorder{}
	dispatchWebSocketEvent([]byte(`{"type":"pin_added","user":"U1","channel_id":"C1","item":{"type":"message"},"event_ts":"1700000000.000100"}`), h)
	dispatchWebSocketEvent([]byte(`{"type":"pin_removed","user":"U1","channel_id":"C2","item":{"type":"message"},"has_pins":false}`), h)
	if want := []string{"C1 true", "C2 false"}; !reflect.DeepEqual(h.events, want) {
		t.Errorf("OnPinsChanged got %v, want %v", h.events, want)
	}
	// A handler that does not hear pins is left alone.
	dispatchWebSocketEvent([]byte(`{"type":"pin_added","channel_id":"C1"}`), &mockEventHandler{})
}

// A "ratelimited" refusal of an app's action or a Home click keeps
// Slack's code, which the failure toast quotes: "<app> failed:
// ratelimited".
func TestAppActions_RatelimitedToastText(t *testing.T) {
	c, _, _ := appActionsServer(t, `{"ok":false,"error":"ratelimited"}`)
	err := c.RunAppAction(context.Background(), "A1", "1", "C1", "1.0", "web-1")
	if got := "Annotate failed: " + err.Error(); got != "Annotate failed: ratelimited" {
		t.Errorf("app action toast %q", got)
	}
	err = c.SendBlockAction(context.Background(), BlockAction{ViewID: "V1"})
	if got := "Colony failed: " + err.Error(); got != "Colony failed: ratelimited" {
		t.Errorf("Home click toast %q", got)
	}
}
