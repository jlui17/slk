package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// MarkAgentThread records that the user marked the thread as an agent
// thread with :agent. Marking a marked thread is a no-op.
func (db *DB) MarkAgentThread(workspaceID, channelID, threadTS string) error {
	_, err := db.conn.Exec(`
		INSERT OR IGNORE INTO agent_thread_marks (workspace_id, channel_id, thread_ts, marked_at)
		VALUES (?, ?, ?, ?)`,
		workspaceID, channelID, threadTS, time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("marking agent thread: %w", err)
	}
	return nil
}

// UnmarkAgentThread removes the thread's :agent mark, if any.
func (db *DB) UnmarkAgentThread(workspaceID, channelID, threadTS string) error {
	_, err := db.conn.Exec(`
		DELETE FROM agent_thread_marks
		WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?`,
		workspaceID, channelID, threadTS,
	)
	if err != nil {
		return fmt.Errorf("unmarking agent thread: %w", err)
	}
	return nil
}

// AgentThreadMarked reports whether the thread carries an :agent mark.
func (db *DB) AgentThreadMarked(workspaceID, channelID, threadTS string) (bool, error) {
	var one int
	err := db.conn.QueryRow(`
		SELECT 1
		FROM agent_thread_marks
		WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?`,
		workspaceID, channelID, threadTS,
	).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("querying agent thread mark: %w", err)
	}
	return true, nil
}
