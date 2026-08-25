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
	System    []struct {
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

func TestLabelSendsRootAndParsesReply(t *testing.T) {
	srv, got := fakeAPI(t, "fix ingest retries\n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	label, err := c.Label(context.Background(), "please fix the ingest retries in colony")
	if err != nil {
		t.Fatalf("Label: %v", err)
	}
	if label != "fix ingest retries" {
		t.Errorf("label = %q, want %q", label, "fix ingest retries")
	}
	if got.Model != "claude-haiku-4-5" {
		t.Errorf("model = %q", got.Model)
	}
	if got.MaxTokens <= 0 || got.MaxTokens > 1024 {
		t.Errorf("max_tokens = %d, want small positive", got.MaxTokens)
	}
	if len(got.System) == 0 || !strings.Contains(got.System[0].Text, "30 characters") {
		t.Errorf("system prompt missing the length rule: %+v", got.System)
	}
	if len(got.Messages) != 1 || got.Messages[0].Role != "user" {
		t.Fatalf("messages = %+v, want one user message", got.Messages)
	}
	if body := got.Messages[0].Content[0].Text; body != "please fix the ingest retries in colony" {
		t.Errorf("user content = %q", body)
	}
}

func TestLabelCapsPromptSize(t *testing.T) {
	srv, got := fakeAPI(t, "big thread")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, err := c.Label(context.Background(), strings.Repeat("x", 10000)); err != nil {
		t.Fatalf("Label: %v", err)
	}
	if n := len(got.Messages[0].Content[0].Text); n > maxRootBytes {
		t.Errorf("user content is %d bytes, want capped at %d", n, maxRootBytes)
	}
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

func TestLabelEmptyCompletionIsError(t *testing.T) {
	srv, _ := fakeAPI(t, "   \n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, err := c.Label(context.Background(), "root"); err == nil {
		t.Fatal("Label returned no error for a blank completion")
	}
}
