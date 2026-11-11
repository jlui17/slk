package tablabel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"text/tabwriter"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
)

// agentStatusEvalRow is one case of testdata/agent_status_eval.json: what
// slk sends the judge for a thread's newest message, and the status it
// should get.
type agentStatusEvalRow struct {
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Note    string   `json:"note,omitempty"`
	Earlier []string `json:"earlier"`
	Message string   `json:"message"`
	Author  Author   `json:"author"`
	// UsersClaudeCodePost is the most recent post of the user's Claude
	// Code in the thread before the newest message, "" when it has not
	// posted.
	UsersClaudeCodePost string  `json:"users_claude_code_post,omitempty"`
	Want                Verdict `json:"want"`
}

// judgeCallRecorder collects the raw replies of the model calls one Judge
// makes, in order. It rides in the Judge's context and is filled by
// recordJudgeCalls, so the eval sees which call decided without Judge
// exposing anything.
type judgeCallRecorder struct {
	mu    sync.Mutex
	calls []judgeCallReply
}

type judgeCallReply struct{ name, reply string }

type judgeCallRecorderKey struct{}

// judgeCallNames maps each call's system prompt to the name this file
// shows for it.
var judgeCallNames = map[string]string{
	workingAgentSystemPrompt:       "first",
	workingUserSystemPrompt:        "first",
	workingOpenAskSystemPrompt:     "open-ask",
	workingEarlierStepSystemPrompt: "earlier-step",
}

// recordJudgeCalls makes every successful model call of c append its raw
// reply text to the judgeCallRecorder in the request's context, if any.
func recordJudgeCalls(c *Client) {
	c.client.Messages.Options = append(c.client.Messages.Options, option.WithMiddleware(
		func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
			rec, _ := req.Context().Value(judgeCallRecorderKey{}).(*judgeCallRecorder)
			var sent struct{ System []struct{ Text string } }
			if rec != nil && req.GetBody != nil {
				if body, err := req.GetBody(); err == nil {
					json.NewDecoder(body).Decode(&sent)
				}
			}
			resp, err := next(req)
			if rec == nil || err != nil || resp.StatusCode != http.StatusOK || len(sent.System) == 0 {
				return resp, err
			}
			raw, readErr := io.ReadAll(resp.Body)
			resp.Body.Close()
			resp.Body = io.NopCloser(bytes.NewReader(raw))
			if readErr != nil {
				return resp, nil
			}
			var got struct{ Content []struct{ Type, Text string } }
			json.Unmarshal(raw, &got)
			name := judgeCallNames[sent.System[0].Text]
			if name == "" {
				name = "unknown"
			}
			for _, block := range got.Content {
				if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
					rec.mu.Lock()
					rec.calls = append(rec.calls, judgeCallReply{name, strings.TrimSpace(block.Text)})
					rec.mu.Unlock()
					break
				}
			}
			return resp, nil
		}))
}

// decidingCallSummary tallies, per call in the order Judge makes them, the
// raw replies of the runs that call decided: the last call a run made, since
// Judge stops at the call that decides (or fails). A single-letter reply
// prints as is, any longer one in single quotes and cut, e.g. "first:u×5" or
// `first:d×5 open-ask:'"Type merge and I will" y'×1,'none'×4`.
func decidingCallSummary(deciders []judgeCallReply) string {
	byCall := map[string]map[string]int{}
	for _, d := range deciders {
		if byCall[d.name] == nil {
			byCall[d.name] = map[string]int{}
		}
		byCall[d.name][d.reply]++
	}
	var parts []string
	for _, name := range []string{"first", "open-ask", "earlier-step", "unknown"} {
		replies := byCall[name]
		var tallies []string
		for reply, n := range replies {
			shown := strings.Join(strings.Fields(reply), " ")
			if len([]rune(shown)) > 1 {
				if r := []rune(shown); len(r) > 48 {
					shown = string(r[:48]) + "…"
				}
				shown = "'" + shown + "'"
			}
			tallies = append(tallies, fmt.Sprintf("%s×%d", shown, n))
		}
		sort.Strings(tallies)
		if len(tallies) > 0 {
			parts = append(parts, name+":"+strings.Join(tallies, ","))
		}
	}
	return strings.Join(parts, " ")
}

// TestRecordJudgeCallsNamesTheDecidingCall runs Judge against a fake API
// whose replies depend on the call, and checks that the recorded calls end
// with the one that decided.
func TestRecordJudgeCallsNamesTheDecidingCall(t *testing.T) {
	replies := map[string]string{
		workingAgentSystemPrompt:       "d",
		workingOpenAskSystemPrompt:     `"Type merge and I will" n`,
		workingEarlierStepSystemPrompt: "y",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req capturedRequest
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "msg_1", "type": "message", "role": "assistant", "model": "m",
			"stop_reason": "end_turn", "usage": map[string]int{"input_tokens": 1, "output_tokens": 1},
			"content": []map[string]string{
				{"type": "thinking", "thinking": "hmm", "signature": "s"},
				{"type": "text", "text": replies[req.System[0].Text]},
			},
		})
	}))
	defer srv.Close()
	judge := newForTest("m", srv.URL)
	recordJudgeCalls(judge)

	rec := &judgeCallRecorder{}
	v, err := judge.Judge(context.WithValue(context.Background(), judgeCallRecorderKey{}, rec), "Done.", []string{"user: go", "agent: on it"}, AuthorAgent, "")
	if err != nil || v != VerdictWorking {
		t.Fatalf("Judge = %v, %v; want working", v, err)
	}
	want := []judgeCallReply{{"first", "d"}, {"open-ask", `"Type merge and I will" n`}, {"earlier-step", "y"}}
	if fmt.Sprint(rec.calls) != fmt.Sprint(want) {
		t.Errorf("calls = %v, want %v", rec.calls, want)
	}
	got := decidingCallSummary(append([]judgeCallReply{want[2], want[2]}, judgeCallReply{"first", "u"}, want[1]))
	if wantSummary := `first:u×1 open-ask:'"Type merge and I will" n'×1 earlier-step:y×2`; got != wantSummary {
		t.Errorf("decidingCallSummary = %q, want %q", got, wantSummary)
	}
}

func loadAgentStatusEvalRows(t *testing.T) []agentStatusEvalRow {
	t.Helper()
	data, err := os.ReadFile("testdata/agent_status_eval.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []agentStatusEvalRow
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatalf("testdata/agent_status_eval.json: %v", err)
	}
	for _, row := range rows {
		if row.Author != AuthorAgent && row.Author != AuthorUser && row.Author != AuthorUsersClaudeCode {
			t.Fatalf("testdata/agent_status_eval.json: %q: author %q is not %q, %q or %q", row.Name, row.Author, AuthorAgent, AuthorUser, AuthorUsersClaudeCode)
		}
	}
	return rows
}

// TestAgentStatusEval judges every dataset row against the real model,
// SLK_AGENT_STATUS_EVAL_RUNS times each (default 1), and fails a row when
// any run misses. SLK_AGENT_STATUS_EVAL_MODEL and SLK_AGENT_STATUS_EVAL_EFFORT
// override the judge model and effort from slk's config.
// tools/agent-status-eval.sh runs it, on this checkout or on any ref.
func TestAgentStatusEval(t *testing.T) {
	_, judge := liveClients(t)
	if m := os.Getenv("SLK_AGENT_STATUS_EVAL_MODEL"); m != "" {
		judge.model = m
	}
	if e := os.Getenv("SLK_AGENT_STATUS_EVAL_EFFORT"); e != "" {
		judge.effort = e
	}
	runs := 1
	if s := os.Getenv("SLK_AGENT_STATUS_EVAL_RUNS"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 {
			t.Fatalf("SLK_AGENT_STATUS_EVAL_RUNS=%q: want a whole number of at least 1", s)
		}
		runs = n
	}
	rows := loadAgentStatusEvalRows(t)
	t.Logf("judge: model %s, effort %s, %d run(s) of %d rows", judge.model, judge.effort, runs, len(rows))

	recordJudgeCalls(judge)
	counts := make([]map[string]int, len(rows))
	deciders := make([][]judgeCallReply, len(rows))
	t.Run("rows", func(t *testing.T) {
		for i, row := range rows {
			t.Run(row.Name, func(t *testing.T) {
				t.Parallel()
				got := map[string]int{}
				var decided []judgeCallReply
				for range runs {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					rec := &judgeCallRecorder{}
					v, err := judge.Judge(context.WithValue(ctx, judgeCallRecorderKey{}, rec), row.Message, row.Earlier, row.Author, row.UsersClaudeCodePost)
					cancel()
					if len(rec.calls) > 0 {
						decided = append(decided, rec.calls[len(rec.calls)-1])
					}
					verdict := string(v)
					if err != nil {
						verdict = "error"
						t.Logf("Judge: %v", err)
					}
					got[verdict]++
				}
				counts[i] = got
				deciders[i] = decided
				if got[string(row.Want)] != runs {
					t.Errorf("want %s in %d of %d runs, got %v", row.Want, got[string(row.Want)], runs, got)
				}
			})
		}
	})

	verdicts := []string{string(VerdictWorking), string(VerdictBlocked), string(VerdictIdle), "error"}
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "row\twant\t%s\t\tdeciding call (raw reply×runs)\n", strings.Join(verdicts, "\t"))
	passed := 0
	for i, row := range rows {
		mark := ""
		if counts[i][string(row.Want)] != runs {
			mark = "  MISS"
		}
		passed += counts[i][string(row.Want)]
		fmt.Fprintf(w, "%s\t%s", row.Name, row.Want)
		for _, v := range verdicts {
			fmt.Fprintf(w, "\t%d", counts[i][v])
		}
		fmt.Fprintf(w, "\t%s\t%s\n", mark, decidingCallSummary(deciders[i]))
	}
	w.Flush()
	t.Logf("model %s, effort %s\n%s\npassed %d of %d case-runs", judge.model, judge.effort, table.String(), passed, len(rows)*runs)
}
