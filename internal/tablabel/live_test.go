package tablabel

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/gammons/slk/internal/config"
	toml "github.com/pelletier/go-toml/v2"
)

// TestRelabelLive hits the real Anthropic API with a root-only transcript,
// what a thread opened before anyone replied sends; set SLK_TABLABEL_LIVE=1
// to run it. The model, effort and key are the ones slk itself uses (see
// liveClients); tools/go.sh carries the gate, the config file and the env
// key into the docker container it runs tests in on Santa hosts.
func TestRelabelLive(t *testing.T) {
	c, _ := liveClients(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	id, label, err := c.Relabel(ctx,
		"justin: colony-562 the flow viewer renders stale runs after a reconnect, can you fix it", nil)
	if err != nil {
		t.Fatalf("Relabel: %v", err)
	}
	t.Logf("live id: %q label: %q", id, label)
	if id != "colony-562" {
		t.Errorf("id = %q, want the root's task id", id)
	}
	if label == "" || utf8.RuneCountInString(label) > 60 {
		t.Errorf("label %q outside expected shape", label)
	}
}

// liveClients returns the tab labeler and the working judge slk builds
// from its config.toml.
func liveClients(t *testing.T) (labeler, judge *Client) {
	t.Helper()
	if os.Getenv("SLK_TABLABEL_LIVE") == "" {
		t.Skip("set SLK_TABLABEL_LIVE=1 to hit the real API")
	}
	cfg := liveConfig(t)
	if cfg.TabNameModel == "" {
		t.Fatal("no model: set herdr.tab_name_model in slk's config.toml")
	}
	apiKey := cfg.ResolveAnthropicAPIKey()
	if apiKey == "" {
		t.Fatal("no API key: set herdr.anthropic_api_key in slk's config.toml, or ANTHROPIC_API_KEY")
	}
	return New(cfg.TabNameModel, cfg.ResolveTabNameEffort(), apiKey),
		New(cfg.ResolveAgentStatusJudgeModel(), cfg.ResolveAgentStatusJudgeEffort(), apiKey)
}

// liveConfig reads the model, effort and key where slk does, reading only
// what it needs: config.toml is decoded without config.Load's workspace
// validation, since a live test is not slk startup. The path restates
// xdgConfig (cmd/slk), which can't be imported. A missing file is an
// empty config.
func liveConfig(t *testing.T) config.Herdr {
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
	return cfg.Herdr
}
