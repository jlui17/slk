package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gammons/slk/internal/config"
)

// The savers no longer create the config directory themselves —
// lockConfig does, since it has to open the sidecar before anything
// reads the config. A first-ever save on a machine with no config
// directory still has to work.
func TestConfigSavers_CreateTheConfigDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "slk", "config.toml")

	if err := saveGlobalTheme(path, "nord"); err != nil {
		t.Fatalf("saveGlobalTheme: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Appearance.Theme != "nord" {
		t.Errorf("theme = %q; want nord", cfg.Appearance.Theme)
	}
}

// A lock slk cannot take must not cancel the save. The write is atomic
// either way, and a user whose filesystem refuses flock would otherwise
// watch their theme silently stop persisting — while the update the
// lock protects is only lost if a second instance saves at that exact
// moment.
func TestConfigSavers_SaveWhenTheLockCannotBeTaken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	// A directory where the sidecar goes: opening it fails, the same
	// shape as a filesystem that refuses the lock.
	if err := os.Mkdir(path+".lock", 0755); err != nil {
		t.Fatal(err)
	}

	if err := saveGlobalTheme(path, "nord"); err != nil {
		t.Fatalf("saveGlobalTheme: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Appearance.Theme != "nord" {
		t.Errorf("theme = %q; want nord", cfg.Appearance.Theme)
	}
}

// herdr.anthropic_api_key is a secret the user typed into config.toml by
// hand. Every saver rewrites that file while slk runs, so none of them may
// drop, move, or duplicate it — on the pass that appends its own section
// or on the pass that updates it.
func TestConfigSavers_LeaveTheHerdrAPIKeyInPlace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	const herdr = "[herdr]\ntab_name_model = \"claude-haiku-4-5\"\nanthropic_api_key = \"sk-ant-secret\"   # mine\n"
	if err := os.WriteFile(path, []byte("[general]\n\n"+herdr), 0600); err != nil {
		t.Fatal(err)
	}

	for pass, theme := range []string{"nord", "dracula"} {
		savers := []struct {
			name string
			save func() error
		}{
			{"saveGlobalTheme", func() error { return saveGlobalTheme(path, theme) }},
			{"saveWorkspaceTheme", func() error { return saveWorkspaceTheme(path, "T01ABCDEF", "T01ABCDEF", "Acme", theme) }},
			{"saveWorkspaceWidth", func() error { return saveWorkspaceWidth(path, "T01ABCDEF", "T01ABCDEF", "Acme", 30+pass) }},
			{"saveWorkspaceVersionTS", func() error { return saveWorkspaceVersionTS(path, "T01ABCDEF", "T01ABCDEF", "Acme", theme) }},
			{"appendWorkspaceConfigBlock", func() error {
				return appendWorkspaceConfigBlock(path, "acme", []string{"T02ABCDEF", "T03ABCDEF"}[pass], "Acme")
			}},
		}
		for _, s := range savers {
			if err := s.save(); err != nil {
				t.Fatalf("pass %d %s: %v", pass, s.name, err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if n := strings.Count(string(data), herdr); n != 1 {
				t.Fatalf("pass %d %s: [herdr] block appears %d times, want 1 verbatim:\n%s", pass, s.name, n, data)
			}
		}
	}

	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	if cfg.Herdr.AnthropicAPIKey != "sk-ant-secret" {
		t.Errorf("anthropic_api_key did not survive the saves")
	}
}
