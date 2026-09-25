package cache

import (
	"strings"
	"time"
)

func (db *DB) migrateFork() error {
	const schema = `
	CREATE TABLE IF NOT EXISTS pane_state (
		pane_key TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		thread_ts TEXT NOT NULL DEFAULT '',
		updated_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS herdr_pane_ids (
		pane_key TEXT PRIMARY KEY,
		current_pane_id TEXT NOT NULL,
		updated_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS herdr_tab_labels (
		pane_key TEXT PRIMARY KEY,
		label TEXT NOT NULL,
		updated_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS thread_sweep_claims (
		workspace_id TEXT PRIMARY KEY,
		claimed_at INTEGER NOT NULL DEFAULT 0,
		completed_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS agent_state_reports (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		reported_at INTEGER NOT NULL DEFAULT 0,
		workspace_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		thread_ts TEXT NOT NULL,
		message_ts TEXT NOT NULL DEFAULT '',
		from_agent INTEGER NOT NULL DEFAULT 0,
		state TEXT NOT NULL,
		source TEXT NOT NULL,
		judge_reply TEXT NOT NULL DEFAULT '',
		judge_model TEXT NOT NULL DEFAULT '',
		judge_prompt_hash TEXT NOT NULL DEFAULT '',
		message_text_hash TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT ''
	);
	CREATE INDEX IF NOT EXISTS idx_agent_state_reports_thread
		ON agent_state_reports(channel_id, thread_ts);

	CREATE INDEX IF NOT EXISTS idx_users_workspace_name
		ON users(workspace_id, display_name, name);

	CREATE TABLE IF NOT EXISTS fork_migrations (
		name TEXT PRIMARY KEY,
		applied_at INTEGER NOT NULL DEFAULT 0
	);
	`
	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}
	if err := db.addColumn(`ALTER TABLE thread_sweep_claims ADD COLUMN completed_at INTEGER NOT NULL DEFAULT 0`); err != nil {
		return err
	}
	// Rows cached by a binary on slack-go v0.23.0 carry table cells decoded
	// as element lists, their raw_text/raw_number content gone. Blank
	// raw_json so those messages render text-only until a refetch rewrites
	// the row.
	return db.runOnce("table-cells-lossy-decode", `
		UPDATE messages SET raw_json = ''
		WHERE raw_json LIKE '%"type":"raw_text","elements"%'
		   OR raw_json LIKE '%"type":"raw_number","elements"%'`)
}

// runOnce applies a data migration the first time this database sees
// name; every slk instance sharing the file runs migrateFork, so the
// statement must tolerate a concurrent first run.
func (db *DB) runOnce(name, stmt string) error {
	var applied int
	if err := db.conn.QueryRow(`SELECT COUNT(*) FROM fork_migrations WHERE name = ?`, name).Scan(&applied); err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	if _, err := db.conn.Exec(stmt); err != nil {
		return err
	}
	_, err := db.conn.Exec(`INSERT OR IGNORE INTO fork_migrations (name, applied_at) VALUES (?, ?)`, name, time.Now().Unix())
	return err
}

// addColumn runs an ALTER TABLE ... ADD COLUMN and treats "duplicate
// column name" as success: a fresh database already has the column from
// CREATE TABLE, and two instances opening an old database at once race
// the same ALTER. Unlike addColumnIfMissing's probe-then-add, this has
// no window between the check and the write.
func (db *DB) addColumn(stmt string) error {
	_, err := db.conn.Exec(stmt)
	if err != nil && strings.Contains(err.Error(), "duplicate column name") {
		return nil
	}
	return err
}
