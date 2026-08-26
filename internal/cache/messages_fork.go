package cache

import (
	"fmt"

	"github.com/gammons/slk/internal/debuglog"
)

// UpsertMessages writes a batch of messages in one transaction, so a
// history fetch of N messages costs one commit instead of N. Row
// semantics match N calls to UpsertMessage — same upsert SQL (the
// equivalence is pinned by TestUpsertMessages_MatchesIndividualUpserts),
// and a failing row is logged and skipped without discarding the rest
// of the batch, mirroring the call sites that ignore per-message
// upsert errors.
func (db *DB) UpsertMessages(msgs []Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		debuglog.Cache("UpsertMessages: begin count=%d ERR=%v", len(msgs), err)
		return fmt.Errorf("upserting message batch: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO messages (ts, channel_id, workspace_id, user_id, text, thread_ts, reply_count, edited_at, is_deleted, raw_json, created_at, subtype)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(ts, channel_id) DO UPDATE SET
			user_id=excluded.user_id,
			text=excluded.text,
			thread_ts=excluded.thread_ts,
			reply_count=excluded.reply_count,
			edited_at=excluded.edited_at,
			is_deleted=excluded.is_deleted,
			raw_json=excluded.raw_json,
			subtype=excluded.subtype
	`)
	if err != nil {
		debuglog.Cache("UpsertMessages: prepare ERR=%v", err)
		return fmt.Errorf("upserting message batch: %w", err)
	}
	defer stmt.Close()

	for _, m := range msgs {
		if _, err := stmt.Exec(m.TS, m.ChannelID, m.WorkspaceID, m.UserID, m.Text, m.ThreadTS,
			m.ReplyCount, m.EditedAt, boolToInt(m.IsDeleted), m.RawJSON, m.CreatedAt, m.Subtype); err != nil {
			debuglog.Cache("UpsertMessages: channel=%s ts=%s ERR=%v", m.ChannelID, m.TS, err)
		}
	}

	if err := tx.Commit(); err != nil {
		debuglog.Cache("UpsertMessages: commit count=%d ERR=%v", len(msgs), err)
		return fmt.Errorf("upserting message batch: %w", err)
	}
	debuglog.Cache("UpsertMessages: count=%d channel=%s", len(msgs), msgs[0].ChannelID)
	return nil
}
