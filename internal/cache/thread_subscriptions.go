package cache

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// thread_subscriptions caches the user's read cursor (last_read) in each
// thread Slack has told slk about: from thread_marked and
// thread_subscribed events, the Threads view's getView pages, and the
// root of a conversations.replies fetch. The thread panel reads it for
// its "── new ──" boundary. It is not a list of the user's followed
// threads: the Threads view comes from Slack page by page, so the table
// holds only the threads slk has seen, and nothing here may treat a
// missing row as "not followed".

// UpsertThreadSubscription inserts or updates a thread_subscriptions
// row. Bumps updated_at to time.Now().Unix() on every call. active is
// Slack's subscription flag from a thread_subscribed /
// thread_unsubscribed event.
func (db *DB) UpsertThreadSubscription(workspaceID, channelID, threadTS, lastRead string, active bool) error {
	if workspaceID == "" || channelID == "" || threadTS == "" {
		return fmt.Errorf("UpsertThreadSubscription: workspace/channel/thread_ts required")
	}
	activeInt := 0
	if active {
		activeInt = 1
	}
	const q = `
INSERT INTO thread_subscriptions
    (workspace_id, channel_id, thread_ts, last_read, active, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT(workspace_id, channel_id, thread_ts) DO UPDATE SET
    last_read  = excluded.last_read,
    active     = excluded.active,
    updated_at = excluded.updated_at
`
	_, err := db.conn.Exec(q, workspaceID, channelID, threadTS, lastRead, activeInt, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upserting thread_subscriptions: %w", err)
	}
	return nil
}

// UpdateThreadLastRead advances (or rewinds) a thread's read cursor
// WITHOUT touching `active` on a row that already exists — the ON
// CONFLICT clause updates last_read and updated_at and nothing else.
// A read cursor is not a subscription change, so it must not be able to
// tombstone a row or resurrect one: `active` is owned by
// thread_subscribed and thread_unsubscribed.
//
// A missing row is inserted with active=1, so callers use it only for a
// thread Slack says the user follows: a thread_marked event whose
// subscription block reports active=true, a getView page, or a replies
// root marked subscribed. slk's own subscriptions.thread.mark echoes
// back as thread_marked, so the event by itself does not prove
// subscription; callers that cannot prove it use
// UpdateThreadLastReadIfExists instead.
func (db *DB) UpdateThreadLastRead(workspaceID, channelID, threadTS, lastRead string) error {
	if workspaceID == "" || channelID == "" || threadTS == "" {
		return fmt.Errorf("UpdateThreadLastRead: workspace/channel/thread_ts required")
	}
	const q = `
INSERT INTO thread_subscriptions
    (workspace_id, channel_id, thread_ts, last_read, active, updated_at)
VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT(workspace_id, channel_id, thread_ts) DO UPDATE SET
    last_read  = excluded.last_read,
    updated_at = excluded.updated_at
`
	if _, err := db.conn.Exec(q, workspaceID, channelID, threadTS, lastRead, time.Now().Unix()); err != nil {
		return fmt.Errorf("updating thread last_read: %w", err)
	}
	return nil
}

// UpdateThreadLastReadIfExists advances a thread's read cursor only when
// a row already exists, and never creates one. Used by the local mark
// path, which fires for ANY thread the user opens — including threads
// they were never subscribed to, whose cursor is not worth storing.
//
// It never touches `active` (so a tombstoned row stays tombstoned) or
// `latest_reply` — unlike UpdateThreadLastRead, which leaves `active`
// alone on an existing row but writes active=1 when it inserts one.
func (db *DB) UpdateThreadLastReadIfExists(workspaceID, channelID, threadTS, lastRead string) error {
	if workspaceID == "" || channelID == "" || threadTS == "" {
		return fmt.Errorf("UpdateThreadLastReadIfExists: workspace/channel/thread_ts required")
	}
	const q = `
UPDATE thread_subscriptions
SET last_read = ?, updated_at = ?
WHERE workspace_id=? AND channel_id=? AND thread_ts=?
`
	if _, err := db.conn.Exec(q, lastRead, time.Now().Unix(), workspaceID, channelID, threadTS); err != nil {
		return fmt.Errorf("updating thread last_read: %w", err)
	}
	return nil
}

// GetThreadLastRead returns a thread's read cursor, or "" when no row
// exists. A missing row is not an error.
func (db *DB) GetThreadLastRead(workspaceID, channelID, threadTS string) (string, error) {
	const q = `SELECT last_read FROM thread_subscriptions
WHERE workspace_id=? AND channel_id=? AND thread_ts=?`
	var lastRead string
	err := db.conn.QueryRow(q, workspaceID, channelID, threadTS).Scan(&lastRead)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading thread last_read: %w", err)
	}
	return lastRead, nil
}
