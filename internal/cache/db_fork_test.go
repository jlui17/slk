package cache

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// TestNew_SetsSynchronousNormalOnAllPoolConnections: synchronous is a
// per-connection pragma like busy_timeout, so it must ride the DSN to
// reach every pooled connection. NORMAL reads back as 1 (FULL, the WAL
// default we are overriding, is 2).
func TestNew_SetsSynchronousNormalOnAllPoolConnections(t *testing.T) {
	db, err := New(filepath.Join(t.TempDir(), "sync.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer db.Close()

	ctx := context.Background()
	const N = 4
	conns := make([]*sql.Conn, 0, N)
	for i := 0; i < N; i++ {
		c, err := db.conn.Conn(ctx)
		if err != nil {
			t.Fatalf("Conn[%d]: %v", i, err)
		}
		conns = append(conns, c)
	}
	for i, c := range conns {
		var mode int
		if err := c.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&mode); err != nil {
			t.Fatalf("conn %d PRAGMA synchronous: %v", i, err)
		}
		if mode != 1 {
			t.Errorf("conn %d synchronous = %d, want 1 (NORMAL)", i, mode)
		}
		c.Close()
	}
}

func TestMigrateFork_AddsUsersNameIndexOnPreExistingDB(t *testing.T) {
	dir := t.TempDir()
	dsn := filepath.Join(dir, "old.db")

	// Simulate a pre-migration database: users table with the original
	// workspace-only index, no (workspace_id, display_name, name) index.
	{
		conn, err := sql.Open("sqlite", dsn)
		if err != nil {
			t.Fatal(err)
		}
		_, err = conn.Exec(`
			CREATE TABLE users (
				id TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL,
				name TEXT NOT NULL,
				display_name TEXT NOT NULL DEFAULT '',
				avatar_url TEXT NOT NULL DEFAULT '',
				presence TEXT NOT NULL DEFAULT 'away',
				is_bot INTEGER NOT NULL DEFAULT 0,
				updated_at INTEGER NOT NULL DEFAULT 0
			);
			CREATE INDEX idx_users_workspace ON users(workspace_id);
			INSERT INTO users (id, workspace_id, name, display_name)
				VALUES ('U1', 'T1', 'zed', 'Zed'), ('U2', 'T1', 'amy', 'Amy');
		`)
		if err != nil {
			t.Fatal(err)
		}
		conn.Close()
	}

	db, err := New(dsn)
	if err != nil {
		t.Fatalf("New on pre-existing db: %v", err)
	}
	defer db.Close()

	var count int
	err = db.conn.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master
		WHERE type='index' AND name='idx_users_workspace_name'
	`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("idx_users_workspace_name not created on pre-existing db")
	}

	// The index exists to kill the temp b-tree sort in ListUsers; pin
	// that the planner actually uses it for ListUsers' exact query.
	rows, err := db.conn.Query(`
		EXPLAIN QUERY PLAN
		SELECT id, workspace_id, name, display_name, avatar_url, presence, is_bot, is_external, updated_at
		FROM users WHERE workspace_id = ? ORDER BY display_name, name
	`, "T1")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, notused int
		var detail string
		if err := rows.Scan(&id, &parent, &notused, &detail); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(detail)
		plan.WriteString("\n")
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "idx_users_workspace_name") {
		t.Errorf("ListUsers plan does not use idx_users_workspace_name:\n%s", plan.String())
	}
	if strings.Contains(plan.String(), "TEMP B-TREE") {
		t.Errorf("ListUsers plan still sorts with a temp b-tree:\n%s", plan.String())
	}

	// Reopening (which re-runs migrateFork) must be a no-op.
	db.Close()
	db2, err := New(dsn)
	if err != nil {
		t.Fatalf("re-opening migrated db: %v", err)
	}
	db2.Close()
}

func TestMigrateForkBlanksLossyTableCellRows(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	rows := map[string]string{
		"1.0": `{"blocks":[{"type":"table","rows":[[{"type":"raw_text","elements":[]}]]}]}`,
		"2.0": `{"blocks":[{"type":"table","rows":[[{"type":"raw_number","elements":null}]]}]}`,
		"3.0": `{"blocks":[{"type":"table","rows":[[{"type":"raw_text","text":"docs"}]]}]}`,
		"4.0": `{"blocks":[{"type":"table","rows":[[{"type":"rich_text","elements":[]}]]}]}`,
	}
	for ts, raw := range rows {
		if err := db.UpsertMessage(Message{TS: ts, ChannelID: "C1", WorkspaceID: "T1", Text: "t", RawJSON: raw}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.conn.Exec(`DELETE FROM fork_migrations WHERE name = 'table-cells-lossy-decode'`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateFork(); err != nil {
		t.Fatal(err)
	}
	for ts, wantBlank := range map[string]bool{"1.0": true, "2.0": true, "3.0": false, "4.0": false} {
		m, err := db.GetMessage("C1", ts)
		if err != nil {
			t.Fatal(err)
		}
		if (m.RawJSON == "") != wantBlank {
			t.Errorf("ts %s raw_json blanked = %v, want %v (%q)", ts, m.RawJSON == "", wantBlank, m.RawJSON)
		}
	}

	// One-shot: a lossy row written after the migration ran stays as is.
	if err := db.UpsertMessage(Message{TS: "5.0", ChannelID: "C1", WorkspaceID: "T1", Text: "t", RawJSON: rows["1.0"]}); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateFork(); err != nil {
		t.Fatal(err)
	}
	if m, _ := db.GetMessage("C1", "5.0"); m.RawJSON == "" {
		t.Error("migration re-ran on a second migrateFork call; it must apply once per database")
	}
}
