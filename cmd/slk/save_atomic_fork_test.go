package main

import (
	"os"
	"path/filepath"
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
