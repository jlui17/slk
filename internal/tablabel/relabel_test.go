package tablabel

import (
	"context"
	"strings"
	"testing"
)

func TestRelabelParsesIDAndLabel(t *testing.T) {
	srv, got := fakeAPI(t, "#1170\nImplement viewer fix\n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	transcript := "justin: brainstorm the design\nClaude: implementing option two"
	id, label, err := c.Relabel(context.Background(), transcript, nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if id != "#1170" || label != "Implement viewer fix" {
		t.Errorf("id, label = %q, %q", id, label)
	}
	if len(got.System) == 0 || !strings.Contains(got.System[0].Text, "the work the thread is about") {
		t.Errorf("system prompt not the ongoing-thread one: %+v", got.System)
	}
	content := got.Messages[0].Content
	if len(content) != 2 {
		t.Fatalf("user content = %+v, want the transcript then the reminder", content)
	}
	if content[0].Text != transcript {
		t.Errorf("transcript block = %q", content[0].Text)
	}
	if content[1].Text != relabelReminder || !strings.Contains(content[1].Text, "exactly two lines") {
		t.Errorf("reminder block = %q", content[1].Text)
	}
}

func TestRelabelNoneMeansNoID(t *testing.T) {
	srv, _ := fakeAPI(t, "None\nBrainstorming retry design")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	id, label, err := c.Relabel(context.Background(), "transcript", nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if id != "" || label != "Brainstorming retry design" {
		t.Errorf("id, label = %q, %q", id, label)
	}
}

func TestRelabelSingleLineIsLabelOnly(t *testing.T) {
	srv, _ := fakeAPI(t, "Implement viewer fix")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	id, label, err := c.Relabel(context.Background(), "transcript", nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if id != "" || label != "Implement viewer fix" {
		t.Errorf("id, label = %q, %q", id, label)
	}
}

func TestRelabelBracketedIDUnwrapped(t *testing.T) {
	srv, _ := fakeAPI(t, "[colony-123]\nFix the viewer")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	id, _, err := c.Relabel(context.Background(), "transcript", nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if id != "colony-123" {
		t.Errorf("id = %q, want brackets stripped", id)
	}
}

func TestRelabelSendsHints(t *testing.T) {
	srv, got := fakeAPI(t, "none\nlabel")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	hints := []string{"task ids look like colony-123", "prefer the PR number over the branch"}
	if _, _, err := c.Relabel(context.Background(), "transcript", hints); err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	// Hints ride in both places: after a long transcript the system
	// prompt's copy alone is too far from the answer to hold.
	system := got.System[0].Text
	content := got.Messages[0].Content
	reminder := content[len(content)-1].Text
	if !strings.HasPrefix(reminder, relabelReminder) {
		t.Errorf("reminder block = %q", reminder)
	}
	for _, h := range hints {
		if !strings.Contains(system, h) {
			t.Errorf("hint %q missing from system prompt:\n%s", h, system)
		}
		if !strings.Contains(reminder, h) {
			t.Errorf("hint %q missing from the reminder after the transcript:\n%s", h, reminder)
		}
	}
}

func TestRelabelCapsPromptSize(t *testing.T) {
	srv, got := fakeAPI(t, "none\nbig thread")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, _, err := c.Relabel(context.Background(), strings.Repeat("x", maxTranscriptBytes+50000), nil); err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if n := len(got.Messages[0].Content[0].Text); n > maxTranscriptBytes {
		t.Errorf("user content is %d bytes, want capped at %d", n, maxTranscriptBytes)
	}
}
