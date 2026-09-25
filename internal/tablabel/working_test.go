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
	if v.Verdict != VerdictWorking {
		t.Errorf("verdict = %v, want VerdictWorking for a w completion", v.Verdict)
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
	if v.Verdict != VerdictIdle {
		t.Errorf("verdict = %v, want VerdictIdle for an n completion", v.Verdict)
	}
	if len(got.System) == 0 || got.System[0].Text != workingUserSystemPrompt {
		t.Errorf("system prompt is not the acked-user prompt: %+v", got.System)
	}
	if body := got.Messages[0].Content[0].Text; !strings.Contains(body, "thanks!") {
		t.Errorf("user content = %q", body)
	}
}

func TestOnlyTheJudgeSetsTemperatureZero(t *testing.T) {
	srv, got := fakeAPI(t, "w")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	if _, err := c.Judge(context.Background(), "on it", true); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if got.Temperature == nil || *got.Temperature != 0 {
		t.Errorf("judge temperature = %v, want an explicit 0", got.Temperature)
	}
	if _, _, err := c.Relabel(context.Background(), "transcript", nil); err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	if got.Temperature != nil {
		t.Errorf("relabel temperature = %v, want it left to the API default", *got.Temperature)
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

func TestJudgeReturnsRawReplyAndHashes(t *testing.T) {
	srv, got := fakeAPI(t, "U, it asked a question")
	defer srv.Close()

	c := newForTest("claude-haiku-4-5", srv.URL)
	j, err := c.Judge(context.Background(), "A or B?", true)
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if j.Verdict != VerdictBlocked || j.Reply != "U, it asked a question" {
		t.Errorf("judgment = %+v, want blocked with the raw reply", j)
	}
	if j.PromptHash != shortHash(workingAgentSystemPrompt) || j.TextHash != shortHash(got.Messages[0].Content[0].Text) {
		t.Errorf("hashes = %q, %q; want those of the agent prompt and the sent text", j.PromptHash, j.TextHash)
	}
	if user, _ := c.Judge(context.Background(), "A or B?", false); user.PromptHash == j.PromptHash {
		t.Error("the two system prompts share a hash")
	}
}

func TestJudgeKeepsUnparseableReply(t *testing.T) {
	srv, _ := fakeAPI(t, "maybe")
	defer srv.Close()

	j, err := newForTest("claude-haiku-4-5", srv.URL).Judge(context.Background(), "hm", true)
	if err == nil {
		t.Fatal("want a parse error")
	}
	if j.Verdict != VerdictIdle || j.Reply != "maybe" || j.PromptHash == "" || j.TextHash == "" {
		t.Errorf("judgment on a parse error = %+v", j)
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
