package tablabel

import (
	"context"
	"strings"
	"testing"
)

func TestRelabelSendsTranscriptWithOngoingPrompt(t *testing.T) {
	srv, got := fakeAPI(t, "Implement viewer fix\n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	transcript := "justin: brainstorm the design\nClaude: implementing option two"
	label, err := c.Relabel(context.Background(), transcript)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if label != "Implement viewer fix" {
		t.Errorf("label = %q", label)
	}
	if len(got.System) == 0 || !strings.Contains(got.System[0].Text, "doing now") {
		t.Errorf("system prompt not the ongoing-thread one: %+v", got.System)
	}
	if body := got.Messages[0].Content[0].Text; body != transcript {
		t.Errorf("user content = %q", body)
	}
}

func TestRelabelCapsPromptSize(t *testing.T) {
	srv, got := fakeAPI(t, "big thread")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, err := c.Relabel(context.Background(), strings.Repeat("x", 20000)); err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if n := len(got.Messages[0].Content[0].Text); n > maxTranscriptBytes {
		t.Errorf("user content is %d bytes, want capped at %d", n, maxTranscriptBytes)
	}
}
