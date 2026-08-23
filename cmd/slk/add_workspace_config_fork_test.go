package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/gammons/slk/internal/config"
)

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
