package slackclient

import (
	"testing"
)

type assistantStatusRecord struct {
	channelID, threadTS, botUserID, status string
}

func (m *mockEventHandler) OnAssistantStatus(channelID, threadTS, botUserID, status string) {
	m.assistantStatuses = append(m.assistantStatuses, assistantStatusRecord{channelID, threadTS, botUserID, status})
}

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
	want := assistantStatusRecord{"C0BS6HBB3R6", "1787202220.875429", "U0AJPPX8SE8", "is thinking…"}
	if handler.assistantStatuses[0] != want {
		t.Errorf("set: got %+v, want %+v", handler.assistantStatuses[0], want)
	}
	want.status = ""
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
