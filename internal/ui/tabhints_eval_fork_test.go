// Offline loop for tuning herdr.tab_name_hints: real thread text, the real
// transcript assembly, the real Relabel call, the real reducer that lands
// the reply on the tab. Skips unless SLK_TABLABEL_LIVE=1. The data (thread
// transcripts, hints files, results) is private Slack text, so it lives in
// the gitignored .luidocs/tab-title-eval/.
//
// Run, with the key slk itself uses (herdr.anthropic_api_key in config.toml,
// else ANTHROPIC_API_KEY; tools/go.sh carries both into docker):
//
//	SLK_TABLABEL_LIVE=1 tools/go.sh test ./internal/ui -run '^TestTabHintsEval$' -count=1 -v \
//	  -exec 'env SLK_TABHINTS_FILE=.luidocs/tab-title-eval/hints/E.txt'
//
// Knobs (pass through -exec 'env K=V ...'):
//
//	SLK_TABHINTS_FILE         hints file, repo-relative
//	SLK_TABHINTS_MODEL        default claude-sonnet-5-5
//	SLK_TABHINTS_EFFORT       default low
//	SLK_TABHINTS_TRANSCRIPTS  transcripts dir under the eval dir, default transcripts
//	SLK_TABHINTS_CACHE        cache rows dir under the eval dir that
//	                          TestTabHintsEvalTranscripts reads, default cache
//	SLK_TABHINTS_REPEATS      calls per transcript, default 1
//	SLK_TABHINTS_ROOT_ONLY    1 sends only each transcript's first line (the
//	                          root): the open-time request's worst case, a
//	                          thread opened before any reply exists
//
// A transcript with a <pane>.review-open file beside it is sent as slk sends
// a !review-open thread's request: tablabel.ReviewOpenHints after the hints
// file's, and the result rendered as "!review ...".
package ui

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/core"
	"github.com/gammons/slk/internal/ids"
	"github.com/gammons/slk/internal/tablabel"
	"github.com/gammons/slk/internal/ui/messages"
	toml "github.com/pelletier/go-toml/v2"
)

const tabHintsEvalDir = "../../.luidocs/tab-title-eval"

// tabHintsEvalApp is an App with herdr tab naming on, capturing every label
// NameTab would receive. users backs userInfo the way the live wiring's
// db.GetUser does (BestName, IsBot).
func tabHintsEvalApp(t *testing.T, users map[string]tabHintsEvalUser) (*App, *[]string) {
	tabNames := &[]string{}
	a := newHarnessApp(t, withApp(func(a *App) {
		a.SetAgentReporter(
			func(string, string, string, AgentState, string) {},
			func(string, string, string, string) {},
			func(label string) { *tabNames = append(*tabNames, label) },
			func(userID string) (string, bool, bool) {
				u, ok := users[userID]
				if !ok {
					return "", false, false
				}
				if u.DisplayName != "" {
					return u.DisplayName, u.IsBot != 0, true
				}
				return u.Name, u.IsBot != 0, true
			},
		)
		a.activeTeamID = "T1"
	}))
	return a, tabNames
}

type tabHintsEvalUser struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
	IsBot       int    `json:"is_bot"`
}

// TestTabHintsEvalTranscripts turns <cache>/threads/<pane>.json (rows
// exported from slk's cache.db, root first) into <transcripts>/<pane>.txt by
// running :retitle itself and capturing what it would send. <cache> is
// SLK_TABHINTS_CACHE (default cache) and <transcripts> SLK_TABHINTS_TRANSCRIPTS
// (default transcripts). A !review-open thread's annotated message is
// answered from <cache>/annotated.json (rows: channel_id, ts, user_id,
// text), and a request :retitle marks review-open also writes an empty
// <pane>.review-open beside the transcript, which TestTabHintsEval reads.
func TestTabHintsEvalTranscripts(t *testing.T) {
	if os.Getenv("SLK_TABHINTS_BUILD") == "" {
		t.Skip("set SLK_TABHINTS_BUILD=1 to rebuild the transcripts")
	}
	cacheDir := filepath.Join(tabHintsEvalDir, envOr("SLK_TABHINTS_CACHE", "cache"))
	outDir := filepath.Join(tabHintsEvalDir, envOr("SLK_TABHINTS_TRANSCRIPTS", "transcripts"))
	var userRows []tabHintsEvalUser
	readJSON(t, filepath.Join(cacheDir, "users.json"), &userRows)
	users := map[string]tabHintsEvalUser{}
	for _, u := range userRows {
		users[u.ID] = u
	}
	var annotated []struct {
		ChannelID string `json:"channel_id"`
		TS        string `json:"ts"`
		UserID    string `json:"user_id"`
		Text      string `json:"text"`
	}
	if _, err := os.Stat(filepath.Join(cacheDir, "annotated.json")); err == nil {
		readJSON(t, filepath.Join(cacheDir, "annotated.json"), &annotated)
	}
	preview := func(_ context.Context, channelID ids.ChannelID, ts ids.MessageTS, _ ids.ThreadTS) (string, string, error) {
		for _, m := range annotated {
			if m.ChannelID == string(channelID) && m.TS == string(ts) {
				u := users[m.UserID]
				return cmp.Or(u.DisplayName, u.Name), m.Text, nil
			}
		}
		return "", "", fmt.Errorf("%s/%s not in annotated.json", channelID, ts)
	}
	paths, _ := filepath.Glob(filepath.Join(cacheDir, "threads/*.json"))
	for _, path := range paths {
		var rows []struct {
			TS     string `json:"ts"`
			UserID string `json:"user_id"`
			Text   string `json:"text"`
		}
		readJSON(t, path, &rows)
		items := make([]messages.MessageItem, len(rows))
		for i, r := range rows {
			items[i] = messages.MessageItem{TS: r.TS, UserID: r.UserID, Text: r.Text, ThreadTS: rows[0].TS}
		}
		a, _ := tabHintsEvalApp(t, users)
		a.SetMessageService(core.NewMessageService(core.MessageServiceFuncs{Preview: preview}))
		var transcript string
		var reviewOpen bool
		a.SetAgentTabRelabeler(func(_, _, _, tr, _ string, _ uint64, ro bool) { transcript, reviewOpen = tr, ro })
		a.threadPanel.SetThread(items[0], items[1:], "C1", items[0].TS)
		a.threadVisible = true
		a.updateAgentThread(items[0], nil, "C1", items[0].TS)
		for _, msg := range drainCmds(executeCommand(a, "retitle")) {
			if m, ok := msg.(agentTabAnnotationMsg); ok {
				if m.text == "" {
					t.Fatalf("%s: the annotated message was not found", path)
				}
				reduceAgentTabRelabel(a, m)
			}
		}
		if transcript == "" {
			t.Fatalf("%s: :retitle sent nothing", path)
		}
		out := filepath.Join(outDir, strings.TrimSuffix(filepath.Base(path), ".json")+".txt")
		if err := os.WriteFile(out, []byte(transcript), 0o644); err != nil {
			t.Fatal(err)
		}
		marker := strings.TrimSuffix(out, ".txt") + ".review-open"
		if err := os.Remove(marker); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		if reviewOpen {
			if err := os.WriteFile(marker, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		fmt.Printf("%s\t%d messages\t%d bytes\tbot=%s\treview-open=%t\n", filepath.Base(out), len(items), len(transcript), a.agentSidebar.thread.botUserID, reviewOpen)
	}
}

func envOr(key, fallback string) string {
	return cmp.Or(os.Getenv(key), fallback)
}

// tabHintsEvalAPIKey finds the key where slk does, reading only what it
// needs: config.toml is decoded without config.Load's workspace validation,
// since an eval is not slk startup. The path restates xdgConfig (cmd/slk),
// which can't be imported. A missing file is an empty config.
func tabHintsEvalAPIKey(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	var cfg config.Config
	data, err := os.ReadFile(filepath.Join(dir, "slk", "config.toml"))
	if err == nil {
		err = toml.Unmarshal(data, &cfg)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read slk config: %v", err)
	}
	return cfg.Herdr.ResolveAnthropicAPIKey()
}

// TestTabHintsEval prints one TSV row per transcript and repeat: pane, mode,
// repeat, the id the deterministic hoist finds in the root, the id and label
// Relabel returned, and the label the tab would get from the open-time
// request (the hoisted id is the reducer's fallback for a model none).
func TestTabHintsEval(t *testing.T) {
	if os.Getenv("SLK_TABLABEL_LIVE") == "" {
		t.Skip("set SLK_TABLABEL_LIVE=1 to hit the real API")
	}
	model := os.Getenv("SLK_TABHINTS_MODEL")
	if model == "" {
		model = "claude-sonnet-5-5"
	}
	effort := os.Getenv("SLK_TABHINTS_EFFORT")
	if effort == "" {
		effort = "low"
	}
	var hints []string
	if path := os.Getenv("SLK_TABHINTS_FILE"); path != "" {
		raw, err := os.ReadFile(filepath.Join("../..", path))
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(raw), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				hints = append(hints, line)
			}
		}
	}
	apiKey := tabHintsEvalAPIKey(t)
	if apiKey == "" {
		t.Fatal("no API key: set herdr.anthropic_api_key in slk's config.toml, or ANTHROPIC_API_KEY")
	}
	client := tablabel.New(model, effort, apiKey)
	dir := os.Getenv("SLK_TABHINTS_TRANSCRIPTS")
	if dir == "" {
		dir = "transcripts"
	}
	repeats := 1
	if n, err := strconv.Atoi(os.Getenv("SLK_TABHINTS_REPEATS")); err == nil && n > 0 {
		repeats = n
	}
	mode := "full"
	if os.Getenv("SLK_TABHINTS_ROOT_ONLY") != "" {
		mode = "root-only"
	}
	paths, _ := filepath.Glob(filepath.Join(tabHintsEvalDir, dir, "*.txt"))
	fmt.Printf("# model=%s hints=%d transcripts=%d dir=%s mode=%s repeats=%d\n", model, len(hints), len(paths), dir, mode, repeats)
	fmt.Printf("# pane\tmode\trepeat\tbytes\thoisted\tmodel id\tmodel label\ttab\n")
	for _, path := range paths {
		pane := strings.Replace(strings.TrimSuffix(filepath.Base(path), ".txt"), "_", ":", 1)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		root, _, _ := strings.Cut(string(raw), "\n")
		transcript := string(raw)
		if mode == "root-only" {
			transcript = root
		}
		hoisted := hoistTaskID(root)
		_, err = os.Stat(strings.TrimSuffix(path, ".txt") + ".review-open")
		reviewOpen := err == nil
		threadHints, threadMode := hints, mode
		if reviewOpen {
			threadHints, threadMode = slices.Concat(hints, tablabel.ReviewOpenHints), mode+",review-open"
		}
		for r := 1; r <= repeats; r++ {
			tabHintsEvalOne(t, client, threadHints, pane, threadMode, r, transcript, hoisted, reviewOpen)
		}
	}
}

func tabHintsEvalOne(t *testing.T, client *tablabel.Client, hints []string, pane, mode string, r int, transcript, hoisted string, reviewOpen bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	id, label, err := client.Relabel(ctx, transcript, hints)
	cancel()
	if err != nil {
		fmt.Printf("%s\t%s\t%d\tERROR\t%v\n", pane, mode, r, err)
		return
	}

	a, tabNames := tabHintsEvalApp(t, map[string]tabHintsEvalUser{"UBOT": {Name: "Claude", IsBot: 1}})
	parent := messages.MessageItem{TS: "100.0", Text: "<@UBOT> placeholder root", UserID: "UHUMAN"}
	a.updateAgentThread(parent, nil, "C1", "100.0")
	before := len(*tabNames)
	reduceAgentTabRelabel(a, AgentTabRelabelMsg{TeamID: "T1", ChannelID: "C1", ThreadTS: "100.0", TaskID: id, FallbackTaskID: hoisted, ReviewOpen: reviewOpen, Label: label})
	final := "(label left unchanged)"
	if len(*tabNames) > before {
		final = (*tabNames)[len(*tabNames)-1]
	}
	if id == "" {
		id = "none"
	}
	if hoisted == "" {
		hoisted = "none"
	}
	fmt.Printf("%s\t%s\t%d\t%d\t%s\t%s\t%s\t%s\n", pane, mode, r, len(transcript), hoisted, id, label, final)
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}
