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
	if len(got.System) == 0 || !strings.Contains(got.System[0].Text, "y if the agent is still working") {
		t.Errorf("system prompt missing the y/n contract: %+v", got.System)
	}
	body := got.Messages[0].Content[0].Text
	if !strings.HasPrefix(body, "Newest message, from the agent:\n") ||
		!strings.Contains(body, "let me go check") {
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
	body := got.Messages[0].Content[0].Text
	if !strings.Contains(body, "from the user") || !strings.Contains(body, "reacted") {
		t.Errorf("user content missing the acked-user framing: %q", body)
	}
}

func TestWorkingCapsMessageSize(t *testing.T) {
	srv, got := fakeAPI(t, "y")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, err := c.Working(context.Background(), strings.Repeat("x", 10000), true); err != nil {
		t.Fatalf("Working: %v", err)
	}
	if n := len(got.Messages[0].Content[0].Text); n > maxWorkingBytes+100 {
		t.Errorf("user content is %d bytes, want capped near %d", n, maxWorkingBytes)
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
