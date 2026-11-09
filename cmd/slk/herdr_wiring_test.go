package main

import (
	"reflect"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func TestHerdrPaneCacheKeysByLaunchID(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	load, save := herdrPaneCache("w1:p1", "pane id", db.GetHerdrPaneID, db.RecordHerdrPaneID)

	if _, ok := load(); ok {
		t.Fatal("empty store must miss")
	}
	if err := save("w2:p2"); err != nil {
		t.Fatal(err)
	}
	if got, ok := load(); !ok || got != "w2:p2" {
		t.Fatalf("load = %q, %v; want %q, true", got, ok, "w2:p2")
	}
	// Both hook parameters are pane-id-shaped strings, so a load/save
	// roundtrip alone also passes with the key and value transposed;
	// pin that the row is keyed by the launch id, not the resolved one.
	if got, ok, err := db.GetHerdrPaneID("w1:p1"); err != nil || !ok || got != "w2:p2" {
		t.Errorf("row under launch id: %q, %v, %v; want %q, true, nil", got, ok, err, "w2:p2")
	}
	if _, ok, _ := db.GetHerdrPaneID("w2:p2"); ok {
		t.Error("row keyed by the resolved id instead of the launch id")
	}
}

func TestResumeLauncher(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	cases := []struct {
		name        string
		openCommand string
		vars        map[string]string
		want        []string
	}{
		{
			name:        "run-docker.sh names its checkout and role",
			openCommand: "/main/tools/run-docker.sh",
			vars:        map[string]string{"SLK_HOST_LAUNCHER": "/main/.claude/worktrees/w/tools/run-docker.sh", "SLK_ROLE": "agent"},
			want:        []string{"env", "SLK_ROLE=agent", "/main/.claude/worktrees/w/tools/run-docker.sh"},
		},
		{
			name:        "outside docker: open_command",
			openCommand: "/opt/slk/run --flag",
			want:        []string{"/opt/slk/run", "--flag"},
		},
		{
			name:        "unset open_command resolves to slk",
			openCommand: "slk",
			want:        []string{"slk"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := resumeLauncher(c.openCommand, env(c.vars))
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("resumeLauncher = %q, want %q", got, c.want)
			}
		})
	}
}
