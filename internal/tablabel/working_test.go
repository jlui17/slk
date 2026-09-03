package tablabel

import (
	"context"
	"strings"
	"testing"
)

func TestWorkingFramesAgentMessage(t *testing.T) {
	srv, got := fakeAPI(t, "y")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	working, err := c.Working(context.Background(), "let me go check the workflow config", true)
	if err != nil {
		t.Fatalf("Working: %v", err)
	}
	if !working {
		t.Error("expected working=true for a y completion")
	}
	if len(got.System) == 0 || got.System[0].Text != workingAgentSystemPrompt {
		t.Errorf("system prompt is not the agent-side prompt: %+v", got.System)
	}
	if body := got.Messages[0].Content[0].Text; !strings.Contains(body, "let me go check") {
		t.Errorf("user content = %q", body)
	}
}

func TestWorkingFramesAckedUserMessage(t *testing.T) {
	srv, got := fakeAPI(t, "n")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	working, err := c.Working(context.Background(), "thanks!", false)
	if err != nil {
		t.Fatalf("Working: %v", err)
	}
	if working {
		t.Error("expected working=false for an n completion")
	}
	if len(got.System) == 0 || got.System[0].Text != workingUserSystemPrompt {
		t.Errorf("system prompt is not the acked-user prompt: %+v", got.System)
	}
	if body := got.Messages[0].Content[0].Text; !strings.Contains(body, "thanks!") {
		t.Errorf("user content = %q", body)
	}
}

func TestWorkingCapsMessageSize(t *testing.T) {
	srv, got := fakeAPI(t, "y")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	long := "HEAD-" + strings.Repeat("x", 10000) + "-TAIL"
	if _, err := c.Working(context.Background(), long, true); err != nil {
		t.Fatalf("Working: %v", err)
	}
	body := got.Messages[0].Content[0].Text
	if n := len(body); n > maxWorkingBytes+100 {
		t.Errorf("user content is %d bytes, want capped near %d", n, maxWorkingBytes)
	}
	if !strings.Contains(body, "HEAD-") || !strings.Contains(body, "-TAIL") {
		t.Errorf("clipped content lost an end: %q…%q", body[:40], body[len(body)-20:])
	}
}

func TestParseWorkingReply(t *testing.T) {
	cases := []struct {
		reply   string
		working bool
		wantErr bool
	}{
		{reply: "y", working: true},
		{reply: "Y\n", working: true},
		{reply: "yes", working: true},
		{reply: "n", working: false},
		{reply: "No — looks finished.", working: false},
		{reply: "maybe", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseWorkingReply(tc.reply)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseWorkingReply(%q) = %v, want error", tc.reply, got)
			}
			continue
		}
		if err != nil || got != tc.working {
			t.Errorf("parseWorkingReply(%q) = %v, %v; want %v", tc.reply, got, err, tc.working)
		}
	}
}
