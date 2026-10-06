package tablabel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWorkingFramesAgentMessage(t *testing.T) {
	srv, got := fakeAPI(t, "w")
	defer srv.Close()

	c := newForTest("claude-sonnet-5-5", srv.URL)
	v, err := c.Judge(context.Background(), "let me go check the workflow config", nil, true)
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

// fakeJudgeAPI answers the nth request with replies[n] and records each one.
func fakeJudgeAPI(t *testing.T, replies ...string) (*httptest.Server, *[]capturedRequest) {
	t.Helper()
	got := &[]capturedRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req capturedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		*got = append(*got, req)
		if len(*got) > len(replies) {
			t.Errorf("request %d, want at most %d", len(*got), len(replies))
			http.Error(w, "unexpected request", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant",
			"model":       "claude-sonnet-5-5",
			"content":     []map[string]any{{"type": "text", "text": replies[len(*got)-1]}},
			"stop_reason": "end_turn",
			"usage":       map[string]any{"input_tokens": 10, "output_tokens": 5},
		})
	}))
	return srv, got
}

// An agent message that reads done alone is asked about again with the
// messages before it: first for an ask the user has not answered, which is
// blocked and ends there, then for a step an agent still holds. Every other
// verdict is the newest message's alone.
func TestWorkingAsksAboutEarlierMessagesOnlyAfterADoneVerdict(t *testing.T) {
	earlier := []string{"user: fix both PRs", "agent: #1 is ready. Type merge and I will merge it. I'm fixing #2."}
	const message = "#1 is merged."
	const earlierContent = "Earlier messages, oldest first:\nuser: fix both PRs\nagent: #1 is ready. Type merge and I will merge it. I'm fixing #2.\n\n"
	wantRequests := []struct{ system, content string }{
		{workingAgentSystemPrompt, "Newest message:\n" + message},
		{workingOpenAskSystemPrompt, earlierContent + openAskNewestHeading + message},
		{workingEarlierStepSystemPrompt, earlierContent + "Newest message:\n" + message},
	}
	for _, tc := range []struct {
		name      string
		replies   []string
		earlier   []string
		fromAgent bool
		want      Verdict
	}{
		{"done, and an earlier ask is open", []string{"d", `"Type merge and I will" y`}, earlier, true, VerdictBlocked},
		{"done, no open ask, and an earlier step is open", []string{"d", "none n", "y"}, earlier, true, VerdictWorking},
		{"done, and nothing earlier is open", []string{"d", "none n", "n"}, earlier, true, VerdictIdle},
		// The model also quotes the newest message, which asks nothing.
		{"done, and the quoted ask is not in an earlier agent message", []string{"d", `"#1 is merged." y`, "n"}, earlier, true, VerdictIdle},
		{"done with no earlier messages", []string{"d"}, nil, true, VerdictIdle},
		{"working", []string{"w"}, earlier, true, VerdictWorking},
		{"blocked", []string{"u"}, earlier, true, VerdictBlocked},
		{"user message", []string{"n"}, earlier, false, VerdictIdle},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, got := fakeJudgeAPI(t, tc.replies...)
			defer srv.Close()
			c := newForTest("claude-sonnet-5-5", srv.URL)
			v, err := c.Judge(context.Background(), message, tc.earlier, tc.fromAgent)
			if err != nil || v != tc.want {
				t.Fatalf("Judge = %v, %v; want %v", v, err, tc.want)
			}
			if len(*got) != len(tc.replies) {
				t.Fatalf("%d requests, want %d", len(*got), len(tc.replies))
			}
			for i, req := range *got {
				want := wantRequests[i]
				if !tc.fromAgent {
					want.system = workingUserSystemPrompt
				}
				if req.System[0].Text != want.system {
					t.Errorf("request %d system prompt = %q", i, req.System[0].Text)
				}
				if body := req.Messages[0].Content[0].Text; body != want.content {
					t.Errorf("request %d user content = %q, want %q", i, body, want.content)
				}
				if req.Thinking.Type != "" || req.OutputConfig.Effort != "low" || req.Temperature != nil {
					t.Errorf("request %d thinking = %q, effort = %q, temperature = %v; want thinking unset, low effort, no temperature", i, req.Thinking.Type, req.OutputConfig.Effort, req.Temperature)
				}
			}
		})
	}
}

func TestParseOpenAsk(t *testing.T) {
	earlier := []string{"user: \"ship it\" when ready?", "agent: *Type \"merge 7\" and I'll merge it.* CI is green."}
	for _, tc := range []struct {
		reply   string
		want    bool
		wantErr bool
	}{
		{reply: `"Type "merge 7" and I'll merge it." y`, want: true},
		{reply: `"type "merge 7" and" Y`, want: true},
		{reply: `"Type "merge 7" and I'll merge it." n`},
		{reply: "none n"},
		{reply: "none, n"},
		{reply: "none"},
		{reply: " None.\n"},
		// Words from the newest message or from the user are no agent ask.
		{reply: `"Merge word is yours." y`},
		{reply: `"ship it" y`},
		{reply: "none y"},
		// An empty quote is in every message, so it names no ask.
		{reply: `"" y`},
		{reply: "", wantErr: true},
		{reply: "the ask is open", wantErr: true},
	} {
		got, err := parseOpenAsk(tc.reply, earlier)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("parseOpenAsk(%q) = %v, %v; want %v, error %v", tc.reply, got, err, tc.want, tc.wantErr)
		}
	}
}

func TestWorkingCapsEachEarlierMessage(t *testing.T) {
	srv, got := fakeJudgeAPI(t, "d", "none n", "y")
	defer srv.Close()

	c := newForTest("claude-sonnet-5-5", srv.URL)
	long := "agent: HEAD-" + strings.Repeat("x", 10000) + "-TAIL"
	if _, err := c.Judge(context.Background(), "done", []string{long, long}, true); err != nil {
		t.Fatalf("Judge: %v", err)
	}
	for _, req := range (*got)[1:] {
		body := req.Messages[0].Content[0].Text
		if n := len(body); n > 2*maxEarlierBytes+400 {
			t.Errorf("user content is %d bytes, want each earlier message capped near %d", n, maxEarlierBytes)
		}
		if strings.Count(body, "HEAD-") != 2 || strings.Count(body, "-TAIL") != 2 {
			t.Errorf("a clipped earlier message lost an end: %q", body)
		}
	}
}

func TestWorkingFramesAckedUserMessage(t *testing.T) {
	srv, got := fakeAPI(t, "n")
	defer srv.Close()

	c := newForTest("claude-sonnet-5-5", srv.URL)
	v, err := c.Judge(context.Background(), "thanks!", nil, false)
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

	c := newForTest("claude-sonnet-5-5", srv.URL)
	long := "HEAD-" + strings.Repeat("x", 10000) + "-TAIL"
	if _, err := c.Judge(context.Background(), long, nil, true); err != nil {
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
