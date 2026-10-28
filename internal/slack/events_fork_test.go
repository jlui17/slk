package slackclient

import (
	"reflect"
	"testing"
)

func TestDispatchAssistantStatus(t *testing.T) {
	handler := &mockEventHandler{}

	// Captured payloads: non-empty status (turn in progress), then the
	// empty-status clear. status_type varies and must not affect dispatch.
	set := []byte(`{"type":"ai_assistant_status","channel_id":"C0BS6HBB3R6","status":"is thinking…","bot_user_id":"U0AJPPX8SE8","thread_ts":"1787202220.875429","status_type":"banner","event_ts":"1787202222.003300","ts":"1787202222.003300"}`)
	clear := []byte(`{"type":"ai_assistant_status","channel_id":"C0BS6HBB3R6","status":"","bot_user_id":"U0AJPPX8SE8","thread_ts":"1787202220.875429","status_type":"typing","event_ts":"1787202321.003500","ts":"1787202321.003500"}`)
	dispatchWebSocketEvent(set, handler)
	dispatchWebSocketEvent(clear, handler)

	if len(handler.assistantStatuses) != 2 {
		t.Fatalf("expected 2 assistant statuses, got %d", len(handler.assistantStatuses))
	}
	want := AssistantStatus{ChannelID: "C0BS6HBB3R6", ThreadTS: "1787202220.875429", BotUserID: "U0AJPPX8SE8", Status: "is thinking…"}
	if handler.assistantStatuses[0] != want {
		t.Errorf("set: got %+v, want %+v", handler.assistantStatuses[0], want)
	}
	want.Status = ""
	if handler.assistantStatuses[1] != want {
		t.Errorf("clear: got %+v, want %+v", handler.assistantStatuses[1], want)
	}
}

func TestDispatch_ThreadMarked_WireCapture_SubscribedOnFullyReadThread(t *testing.T) {
	// Wire capture: a second Slack client reading a 3-reply thread to
	// its end (last_read == the newest reply's ts), observed 2026-08-21
	// on a live slk WS connection. active=true on a fully-read thread is
	// the exact frame the old active->read inversion misclassified.
	handler := &mockEventHandler{}
	data := []byte(`{"type":"thread_marked","subscription":{"type":"thread","channel":"C0BS6HBB3R6","thread_ts":"1787352842.910909","date_create":1787352862,"active":true,"last_read":"1787352903.834189"},"event_ts":"1787353529.072900"}`)
	dispatchWebSocketEvent(data, handler)

	if len(handler.threadMarks) != 1 {
		t.Fatalf("expected 1 threadMark, got %d", len(handler.threadMarks))
	}
	got := handler.threadMarks[0]
	if got.channelID != "C0BS6HBB3R6" || got.threadTS != "1787352842.910909" || got.lastRead != "1787352903.834189" {
		t.Errorf("unexpected: %+v", got)
	}
	if !got.subscribed {
		t.Error("expected subscribed=true passed through verbatim")
	}
}

// A message Slack shows to this user alone arrives with is_ephemeral and
// goes to OnEphemeralMessage, never OnMessage, which persists what it
// gets. The frame is the shape of the Colony app's chat.postEphemeral in
// a thread.
func TestDispatchEphemeralMessage(t *testing.T) {
	handler := &mockEventHandler{}
	data := []byte(`{"type":"message","subtype":"bot_message","is_ephemeral":true,"channel":"C1","user":"U0BOT","bot_id":"B1","text":"Your annotation was sent.","ts":"1787400000.000200","thread_ts":"1787300000.000100",
		"blocks":[{"type":"actions","block_id":"review","elements":[{"type":"button","action_id":"open_review","text":{"type":"plain_text","text":"Complete review"},"url":"https://x.slack.com/archives/C1/p1787300000000100"}]}]}`)
	dispatchWebSocketEvent(data, handler)

	if len(handler.messages) != 0 {
		t.Errorf("OnMessage got %q; an ephemeral must not reach it", handler.messages)
	}
	if len(handler.ephemerals) != 1 {
		t.Fatalf("OnEphemeralMessage called %d times, want 1", len(handler.ephemerals))
	}
	got := handler.ephemerals[0]
	if got.ChannelID != "C1" || got.UserID != "U0BOT" || got.TS != "1787400000.000200" || got.Text != "Your annotation was sent." ||
		got.ThreadTS != "1787300000.000100" || got.Subtype != "bot_message" || got.BotID != "B1" || len(got.Blocks.BlockSet) != 1 {
		t.Errorf("OnEphemeralMessage got %+v", got)
	}
}

// An app replacing its ephemeral (replace_original, /giphy's Shuffle)
// arrives as message_changed; it goes to OnEphemeralMessage, with the ts
// it replaces and its buttons' action ids, whichever level carries
// is_ephemeral. Upstream drops it.
func TestDispatchEphemeralMessageChanged(t *testing.T) {
	for _, frame := range []string{
		`{"type":"message","subtype":"message_changed","channel":"C1","message":{"is_ephemeral":true,"bot_id":"B1","text":"replaced","ts":"1787400000.000200",
			"attachments":[{"id":1,"actions":[{"id":"7","name":"x","text":"X","type":"button"}]}]}}`,
		`{"type":"message","subtype":"message_changed","is_ephemeral":true,"channel":"C1","message":{"bot_id":"B1","text":"replaced","ts":"1787400000.000200",
			"attachments":[{"id":1,"actions":[{"id":"7","name":"x","text":"X","type":"button"}]}]}}`,
	} {
		handler := &mockEventHandler{}
		dispatchWebSocketEvent([]byte(frame), handler)
		if len(handler.messages) != 0 {
			t.Errorf("%s: OnMessage got %q; an ephemeral must not reach it", frame, handler.messages)
		}
		if len(handler.ephemerals) != 1 {
			t.Fatalf("%s: OnEphemeralMessage called %d times, want 1", frame, len(handler.ephemerals))
		}
		got := handler.ephemerals[0]
		if got.ChannelID != "C1" || got.TS != "1787400000.000200" || got.Text != "replaced" || got.BotID != "B1" {
			t.Errorf("%s: OnEphemeralMessage got %+v", frame, got)
		}
		if want := [][]string{{"7"}}; !reflect.DeepEqual(got.ActionIDs, want) {
			t.Errorf("%s: ActionIDs = %v, want %v", frame, got.ActionIDs, want)
		}
	}
}
