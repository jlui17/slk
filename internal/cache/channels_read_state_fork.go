package cache

import (
	"fmt"
	"strconv"
	"strings"
)

// countedMentionsKeep is how long, in seconds of message ts, a counted
// message stays in counted_mentions. Every slk applies a message event
// within moments of the others, so a day is far more than any lags.
const countedMentionsKeep = 24 * 60 * 60

// CountChannelMention adds one to the channel's mention count for the
// message at ts, once however many times it is called for that message.
// Every slk on the machine shares this cache and gets every message
// event, so the call comes once per slk. counted_mentions records each
// message counted and outlives a read (SetChannelMentionCount), so a
// slk that applies the message after the channel was read does not
// badge it again.
func (db *DB) CountChannelMention(channelID, ts string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("counting channel mention: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.Exec(`INSERT OR IGNORE INTO counted_mentions (channel_id, ts) VALUES (?, ?)`, channelID, ts)
	if err != nil {
		return fmt.Errorf("counting channel mention: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil || n == 0 {
		return err
	}
	if _, err := tx.Exec(`UPDATE channels SET mention_count = mention_count + 1 WHERE id = ?`, channelID); err != nil {
		return fmt.Errorf("counting channel mention: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM counted_mentions WHERE channel_id = ? AND ts < ?`, channelID, countedMentionsCutoff(ts)); err != nil {
		return fmt.Errorf("pruning counted mentions: %w", err)
	}
	return tx.Commit()
}

// countedMentionsCutoff is the ts countedMentionsKeep before ts. Slack
// ts values share one width ("1700000000.000100"), so they compare as
// strings, and the bare seconds sort before every ts of that second.
func countedMentionsCutoff(ts string) string {
	sec, _, _ := strings.Cut(ts, ".")
	n, err := strconv.ParseInt(sec, 10, 64)
	if err != nil {
		return ""
	}
	return strconv.FormatInt(n-countedMentionsKeep, 10)
}
