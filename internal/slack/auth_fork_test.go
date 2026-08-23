package slackclient

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// Remint rewrites every token file on every launch, so a second slk
// instance can be loading one exactly as it is rewritten. A
// truncate-then-write leaves that reader with unparseable JSON, and
// List() then silently drops the workspace as corrupted.
//
// The padding widens the window a truncating write would leave: a real
// token file is a few hundred bytes, small enough that the race would
// rarely reproduce and the test would pass against the broken write.
func TestSaveToken_ReaderNeverSeesPartialToken(t *testing.T) {
	dir := t.TempDir()
	s := NewTokenStore(dir)
	padding := strings.Repeat("x", 256*1024)

	if err := s.Save(Token{AccessToken: "xoxc-" + padding, TeamID: "T1", TeamName: "Acme"}); err != nil {
		t.Fatalf("seed Save: %v", err)
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	saveErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !stop.Load(); i++ {
			tok := Token{AccessToken: "xoxc-" + padding, Cookie: "c", TeamID: "T1", TeamName: "Acme"}
			if err := s.Save(tok); err != nil {
				saveErr <- err
				return
			}
		}
	}()

	for i := 0; i < 300; i++ {
		got, err := s.Load("T1")
		if err != nil {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("read %d could not load the token: %v", i, err)
		}
		if got.TeamName != "Acme" {
			stop.Store(true)
			wg.Wait()
			t.Fatalf("read %d loaded an incomplete token: %+v", i, got)
		}
	}

	stop.Store(true)
	wg.Wait()
	select {
	case err := <-saveErr:
		t.Fatalf("Save: %v", err)
	default:
	}
}

// A failed or interrupted save must not leave a temp file next to the
// tokens, and the saved file keeps the owner-only mode.
func TestSaveToken_LeavesOneOwnerOnlyFile(t *testing.T) {
	dir := t.TempDir()
	s := NewTokenStore(dir)
	if err := s.Save(Token{AccessToken: "xoxc-1", TeamID: "T1", TeamName: "Acme"}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "T1.json" {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("token directory contains %v; want only T1.json", names)
	}

	fi, err := os.Stat(filepath.Join(dir, "T1.json"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if got := fi.Mode().Perm(); got != 0600 {
		t.Errorf("mode = %o; want 600", got)
	}
}
