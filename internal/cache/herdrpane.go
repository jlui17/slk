package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RecordHerdrPaneID upserts the herdr pane's last-resolved public pane
// id. paneKey is the launch environment's HERDR_PANE_ID; the value is
// the public id the pane currently answers to, which drifts from the
// key when herdr moves the pane across workspaces.
func (db *DB) RecordHerdrPaneID(paneKey, currentPaneID string) error {
	now := time.Now().Unix()
	_, err := db.conn.Exec(`
		INSERT INTO herdr_pane_ids (pane_key, current_pane_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(pane_key)
		DO UPDATE SET current_pane_id = excluded.current_pane_id,
			updated_at = excluded.updated_at`,
		paneKey, currentPaneID, now,
	)
	if err != nil {
		return fmt.Errorf("recording herdr pane id: %w", err)
	}
	return nil
}

// RecordHerdrTabLabel upserts the last tab label slk set for the pane, the
// durable half of tab-label ownership: herdr's own token is wiped by a
// herdr restart while the label survives, and this row is what proves the
// surviving label is slk's to rename.
func (db *DB) RecordHerdrTabLabel(paneKey, label string) error {
	now := time.Now().Unix()
	_, err := db.conn.Exec(`
		INSERT INTO herdr_tab_labels (pane_key, label, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(pane_key)
		DO UPDATE SET label = excluded.label,
			updated_at = excluded.updated_at`,
		paneKey, label, now,
	)
	if err != nil {
		return fmt.Errorf("recording herdr tab label: %w", err)
	}
	return nil
}

// GetHerdrTabLabel returns the last tab label slk set for the pane; ok is
// false when no slk run in this pane has ever renamed its tab.
func (db *DB) GetHerdrTabLabel(paneKey string) (string, bool, error) {
	var label string
	err := db.conn.QueryRow(`
		SELECT label
		FROM herdr_tab_labels
		WHERE pane_key = ?`,
		paneKey,
	).Scan(&label)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying herdr tab label: %w", err)
	}
	return label, true, nil
}

// GetHerdrPaneID returns the last-resolved public pane id for paneKey;
// ok is false when no slk run has ever resolved this pane.
func (db *DB) GetHerdrPaneID(paneKey string) (string, bool, error) {
	var currentPaneID string
	err := db.conn.QueryRow(`
		SELECT current_pane_id
		FROM herdr_pane_ids
		WHERE pane_key = ?`,
		paneKey,
	).Scan(&currentPaneID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("querying herdr pane id: %w", err)
	}
	return currentPaneID, true, nil
}
