package cache

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
	`
	_, err := db.conn.Exec(schema)
	return err
}
