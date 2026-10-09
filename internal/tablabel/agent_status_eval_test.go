package tablabel

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"text/tabwriter"
	"time"
)

// agentStatusEvalRow is one case of testdata/agent_status_eval.json: what
// slk sends the judge for a thread's newest message, and the status it
// should get.
type agentStatusEvalRow struct {
	Name      string   `json:"name"`
	Source    string   `json:"source"`
	Note      string   `json:"note,omitempty"`
	Earlier   []string `json:"earlier"`
	Message   string   `json:"message"`
	FromAgent bool     `json:"from_agent"`
	Want      Verdict  `json:"want"`
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

	counts := make([]map[string]int, len(rows))
	t.Run("rows", func(t *testing.T) {
		for i, row := range rows {
			t.Run(row.Name, func(t *testing.T) {
				t.Parallel()
				got := map[string]int{}
				for range runs {
					ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
					v, err := judge.Judge(ctx, row.Message, row.Earlier, row.FromAgent)
					cancel()
					verdict := string(v)
					if err != nil {
						verdict = "error"
						t.Logf("Judge: %v", err)
					}
					got[verdict]++
				}
				counts[i] = got
				if got[string(row.Want)] != runs {
					t.Errorf("want %s in %d of %d runs, got %v", row.Want, got[string(row.Want)], runs, got)
				}
			})
		}
	})

	verdicts := []string{string(VerdictWorking), string(VerdictBlocked), string(VerdictIdle), "error"}
	var table strings.Builder
	w := tabwriter.NewWriter(&table, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "row\twant\t%s\t\n", strings.Join(verdicts, "\t"))
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
		fmt.Fprintf(w, "\t%s\n", mark)
	}
	w.Flush()
	t.Logf("model %s, effort %s\n%s\npassed %d of %d case-runs", judge.model, judge.effort, table.String(), passed, len(rows)*runs)
}
