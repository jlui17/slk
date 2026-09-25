package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// Close must write every queued row, in the order recorded.
func TestAgentStateReportWriterKeepsOrderAndFlushesOnClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cache.db")
	db, err := cache.New(path)
	if err != nil {
		t.Fatalf("opening db: %v", err)
	}
	defer db.Close()

	w := newAgentStateReportWriter(db)
	want := []string{"working", "blocked", "idle"}
	for _, state := range want {
		w.Record(cache.AgentStateReport{WorkspaceID: "T1", ChannelID: "C1", ThreadTS: "100.0", State: state, Source: "judge"})
	}
	w.Close()

	// cache.DB has no reader for the log (the report script reads it with
	// sqlite3), so the test reads the file itself.
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	rows, err := conn.Query(`SELECT state FROM agent_state_reports ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var state string
		if err := rows.Scan(&state); err != nil {
			t.Fatal(err)
		}
		got = append(got, state)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("states = %v, want %v", got, want)
	}
}
