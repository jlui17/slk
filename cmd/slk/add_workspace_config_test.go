package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gammons/slk/internal/config"
)

func TestUniqueSlug(t *testing.T) {
	existing := map[string]bool{"acme": true, "acme-2": true}
	if got := uniqueSlug("acme", existing); got != "acme-3" {
		t.Errorf("uniqueSlug = %q, want acme-3", got)
	}
	if got := uniqueSlug("fresh", existing); got != "fresh" {
		t.Errorf("uniqueSlug = %q, want fresh", got)
	}
}

func TestUniqueSlugEmptyInputUsesFallback(t *testing.T) {
	existing := map[string]bool{}
	if got := uniqueSlug("", existing); got != "workspace" {
		t.Errorf("uniqueSlug(\"\") = %q, want workspace", got)
	}
}

func TestAppendWorkspaceConfigBlockNewFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := appendWorkspaceConfigBlock(path, "work", "T01ABCDEF", "ACME Corp"); err != nil {
		t.Fatalf("append: %v", err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	ws, ok := cfg.Workspaces["work"]
	if !ok || ws.TeamID != "T01ABCDEF" {
		t.Errorf("workspace not loadable: %+v %v", ws, ok)
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "# ACME Corp") {
		t.Errorf("expected '# ACME Corp' comment, got:\n%s", got)
	}
}

func TestAppendWorkspaceConfigBlockAppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	initial := `[appearance]
theme = "dracula"
`
	if err := os.WriteFile(path, []byte(initial), 0644); err != nil {
		t.Fatal(err)
	}
	if err := appendWorkspaceConfigBlock(path, "work", "T01ABCDEF", "ACME"); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, _ := os.ReadFile(path)
	s := string(got)
	if !strings.Contains(s, "[appearance]") {
		t.Errorf("existing config clobbered, got:\n%s", s)
	}
	if !strings.Contains(s, "[workspaces.work]") || !strings.Contains(s, `team_id = "T01ABCDEF"`) {
		t.Errorf("workspace block not appended, got:\n%s", s)
	}
}

// Two --add-workspace runs against one config must not both claim the
// same slug: go-toml rejects a duplicate [workspaces.X] table, and
// config.Load failing is a hard startup error for slk.
func TestAppendWorkspaceConfigBlockConcurrentSameNameGetsDistinctSlugs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")

	const runs = 8
	var wg sync.WaitGroup
	errs := make(chan error, runs)
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			teamID := fmt.Sprintf("T%07d", i)
			if err := appendWorkspaceConfigBlock(path, "acme", teamID, "ACME"); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("append: %v", err)
	}

	cfg, err := config.Load(path)
	if err != nil {
		raw, _ := os.ReadFile(path)
		t.Fatalf("config.Load after %d concurrent appends: %v\n%s", runs, err, raw)
	}
	if len(cfg.Workspaces) != runs {
		t.Errorf("config has %d workspaces; want %d", len(cfg.Workspaces), runs)
	}
	teamIDs := make(map[string]bool, runs)
	for _, ws := range cfg.Workspaces {
		teamIDs[ws.TeamID] = true
	}
	if len(teamIDs) != runs {
		t.Errorf("config has %d distinct team IDs; want %d", len(teamIDs), runs)
	}
}
