package tablabel

import (
	"context"
	"strings"
	"testing"
)

func TestWorkingFramesAgentMessage(t *testing.T) {
	srv, got := fakeAPI(t, "w")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	v, err := c.Judge(context.Background(), "let me go check the workflow config", true)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if v != VerdictWorking {
		t.Errorf("verdict = %v, want VerdictWorking for a w completion", v)
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
	v, err := c.Judge(context.Background(), "thanks!", false)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if v != VerdictIdle {
		t.Errorf("verdict = %v, want VerdictIdle for an n completion", v)
	}
	if len(got.System) == 0 || got.System[0].Text != workingUserSystemPrompt {
		t.Errorf("system prompt is not the acked-user prompt: %+v", got.System)
	}
	if body := got.Messages[0].Content[0].Text; !strings.Contains(body, "thanks!") {
		t.Errorf("user content = %q", body)
	}
}

func TestWorkingCapsMessageSize(t *testing.T) {
	srv, got := fakeAPI(t, "w")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	long := "HEAD-" + strings.Repeat("x", 10000) + "-TAIL"
	if _, err := c.Judge(context.Background(), long, true); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	body := got.Messages[0].Content[0].Text
	if n := len(body); n > maxWorkingBytes+100 {
		t.Errorf("user content is %d bytes, want capped near %d", n, maxWorkingBytes)
	}
	if !strings.Contains(body, "HEAD-") || !strings.Contains(body, "-TAIL") {
		t.Errorf("clipped content lost an end: %q…%q", body[:40], body[len(body)-20:])
	}
}

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		reply   string
		letters map[byte]Verdict
		want    Verdict
		wantErr bool
	}{
		{reply: "w", letters: agentVerdictLetters, want: VerdictWorking},
		{reply: "U\n", letters: agentVerdictLetters, want: VerdictBlocked},
		{reply: "d — looks finished.", letters: agentVerdictLetters, want: VerdictIdle},
		{reply: "y", letters: agentVerdictLetters, wantErr: true},
		{reply: "yes", letters: userVerdictLetters, want: VerdictWorking},
		{reply: "No — just a thanks.", letters: userVerdictLetters, want: VerdictIdle},
		{reply: "u", letters: userVerdictLetters, wantErr: true},
		{reply: "", letters: userVerdictLetters, wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseVerdict(tc.reply, tc.letters)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseVerdict(%q) = %v, want error", tc.reply, got)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Errorf("parseVerdict(%q) = %v, %v; want %v", tc.reply, got, err, tc.want)
		}
	}
}
