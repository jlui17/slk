package cache

import "time"

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
		claimed_at INTEGER NOT NULL DEFAULT 0
	);

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
