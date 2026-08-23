package cache

import (
	"fmt"

	"github.com/gammons/slk/internal/debuglog"
)

// ThreadNewestActivity returns the ts of the newest known activity in a
// thread: the max of the subscription row's latest_reply watermark (from
// subscriptions.thread.getView) and the newest cached message in the
// thread, parent row included. "" when the thread is entirely unknown.
// Built from the same inputs as ListSubscribedThreads' Unread column,
// with one divergence: the list additionally suppresses a self-authored
// newest message.
func (db *DB) ThreadNewestActivity(workspaceID, channelID, threadTS string) (string, error) {
	const q = `
SELECT
    COALESCE((SELECT latest_reply FROM thread_subscriptions
              WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?), ''),
    COALESCE((SELECT MAX(ts) FROM messages
              WHERE channel_id = ? AND (thread_ts = ? OR ts = ?) AND is_deleted = 0), '')
`
	var latestReply, cachedMaxTS string
	if err := db.conn.QueryRow(q, workspaceID, channelID, threadTS,
		channelID, threadTS, threadTS).Scan(&latestReply, &cachedMaxTS); err != nil {
		return "", fmt.Errorf("querying thread newest activity: %w", err)
	}
	if cachedMaxTS > latestReply {
		return cachedMaxTS, nil
	}
	return latestReply, nil
}

// retractLatestReply: a deleted message can't stay a thread's
// newest-activity watermark: with latest_reply pointing at it, a
// read-to-end at the surviving newest reply computes as unread until
// the next getView reconcile. Clear the watermark to unknown; unread
// computations fall back to the surviving cached messages.
func (db *DB) retractLatestReply(channelID, ts string) error {
	if _, err := db.conn.Exec(
		`UPDATE thread_subscriptions SET latest_reply = '' WHERE channel_id = ? AND latest_reply = ?`,
		channelID, ts,
	); err != nil {
		debuglog.Cache("DeleteMessage: retract latest_reply %s/%s ERR=%v", channelID, ts, err)
		return fmt.Errorf("retracting latest_reply: %w", err)
	}
	return nil
}
