package cache

import (
	"fmt"
)

// ThreadNewestActivity returns the ts of the newest cached message in a
// thread, parent row included, or "" when none is cached. The agent
// thread sidebar compares a thread_marked cursor against it to tell a
// read from a mark-unread; its thread is open or live-cached, so the
// cache holds the newest reply.
func (db *DB) ThreadNewestActivity(channelID, threadTS string) (string, error) {
	var newest string
	if err := db.conn.QueryRow(threadNewestActivityQuery, channelID, threadTS, threadTS).Scan(&newest); err != nil {
		return "", fmt.Errorf("querying thread newest activity: %w", err)
	}
	return newest, nil
}

// The unary + on channel_id keeps SQLite on the thread_ts and ts
// indexes instead of reading every cached message in the channel.
const threadNewestActivityQuery = `
SELECT COALESCE((SELECT MAX(ts) FROM messages
                 WHERE +channel_id = ? AND (thread_ts = ? OR ts = ?) AND is_deleted = 0), '')
`
