package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// PaneState is what a pane restores on relaunch: the channel that was
// open and, when the thread panel was visible, its thread. Empty
// ThreadTS means no thread panel.
type PaneState struct {
	WorkspaceID string
	ChannelID   string
	ThreadTS    string
}

// RecordPaneState upserts the pane's current open state. paneKey
// identifies the pane across restarts (the herdr pane ID, or "default"
// outside herdr).
func (db *DB) RecordPaneState(paneKey string, s PaneState) error {
	now := time.Now().Unix()
	_, err := db.conn.Exec(`
		INSERT INTO pane_state (pane_key, workspace_id, channel_id, thread_ts, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(pane_key)
		DO UPDATE SET workspace_id = excluded.workspace_id,
			channel_id = excluded.channel_id,
			thread_ts = excluded.thread_ts,
			updated_at = excluded.updated_at`,
		paneKey, s.WorkspaceID, s.ChannelID, s.ThreadTS, now,
	)
	if err != nil {
		return fmt.Errorf("recording pane state: %w", err)
	}
	return nil
}

// GetPaneState returns the persisted state for paneKey; ok is false
// when the pane has never recorded one.
func (db *DB) GetPaneState(paneKey string) (PaneState, bool, error) {
	var s PaneState
	err := db.conn.QueryRow(`
		SELECT workspace_id, channel_id, thread_ts
		FROM pane_state
		WHERE pane_key = ?`,
		paneKey,
	).Scan(&s.WorkspaceID, &s.ChannelID, &s.ThreadTS)
	if errors.Is(err, sql.ErrNoRows) {
		return PaneState{}, false, nil
	}
	if err != nil {
		return PaneState{}, false, fmt.Errorf("querying pane state: %w", err)
	}
	return s, true, nil
}
