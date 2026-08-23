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
