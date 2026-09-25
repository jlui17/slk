package tablabel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"
)

type capturedRequest struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	// Temperature is nil when the request leaves it to the API's default.
	Temperature *float64 `json:"temperature"`
	Thinking    struct {
		Type string `json:"type"`
	} `json:"thinking"`
	System []struct {
		Text string `json:"text"`
	} `json:"system"`
	Messages []struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

// fakeAPI serves a canned Messages API success and records the last request.
func fakeAPI(t *testing.T, responseText string) (*httptest.Server, *capturedRequest) {
	t.Helper()
	got := &capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*got = capturedRequest{}
		if err := json.NewDecoder(r.Body).Decode(got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		resp := map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model":       "claude-haiku-4-5",
			"content":     []map[string]any{{"type": "text", "text": responseText}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		}
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			t.Errorf("encode response: %v", err)
		}
	}))
	return srv, got
}

func TestClipNeverSplitsARune(t *testing.T) {
	// "é" is 2 bytes; an odd cap lands mid-rune and must back off to the
	// boundary instead of emitting invalid UTF-8.
	s := strings.Repeat("é", 10)
	for max := 0; max <= len(s); max++ {
		got := clip(s, max)
		if len(got) > max {
			t.Fatalf("clip(%d) = %d bytes", max, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("clip(%d) = %q, invalid UTF-8", max, got)
		}
	}
}

func TestRelabelEmptyCompletionIsError(t *testing.T) {
	srv, _ := fakeAPI(t, "   \n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, _, err := c.Relabel(context.Background(), "transcript", nil); err == nil {
		t.Fatal("Relabel returned no error for a blank completion")
	}
}
