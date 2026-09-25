#!/usr/bin/env bash
# Report on the agent states slk showed in herdr (the agent_state_reports
# log in cache.db), joined to the cached messages, to find statuses that
# were wrong. Only as good as the cache: a thread's later messages are seen
# only if some slk cached them.
#
# Read-only: the database is copied out of the volume through a :ro mount
# and only the copy is opened. Copying a live WAL database can catch it
# mid-write; if sqlite3 reports a malformed database, run it again.
#
# Usage: tools/agent-state-report.sh [volume | path/to/cache.db]
#   (default: slk-test-state, the user sessions' volume)
set -euo pipefail

src=${1:-slk-test-state}
image=slk-go:1.26

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

if [ -f "$src" ]; then
  for ext in "" -wal -shm; do
    if [ -f "$src$ext" ]; then cp "$src$ext" "$tmp/cache.db$ext"; fi
  done
else
  docker run --rm -v "$src":/v:ro "$image" \
    sh -c 'cd /v/xdg/data/slk && tar -cf - cache.db*' | tar -xf - -C "$tmp"
fi
db="$tmp/cache.db"

if [ -z "$(sqlite3 "$db" "SELECT 1 FROM sqlite_master WHERE name = 'agent_state_reports'")" ]; then
  echo "no agent_state_reports table in $src: no slk with the agent-state log has run against it yet" >&2
  exit 1
fi

sqlite3 "$db" <<'SQL'
.headers on
.mode column

-- One row per report that has a message, with the next cached message in
-- its thread (a reply's thread_ts is the root's ts, and Slack ts strings
-- order lexically).
CREATE TEMP VIEW report AS
SELECT r.*,
  datetime(r.reported_at, 'unixepoch', 'localtime') AS reported,
  (SELECT m.ts FROM messages m
    WHERE m.channel_id = r.channel_id AND m.thread_ts = r.thread_ts
      AND m.ts > r.message_ts AND m.is_deleted = 0
    ORDER BY m.ts LIMIT 1) AS next_ts
FROM agent_state_reports r
WHERE r.message_ts != '';

CREATE TEMP VIEW report_next AS
SELECT r.*,
  COALESCE(u.is_bot, next.subtype = 'bot_message') AS next_is_bot,
  CAST(r.next_ts AS INTEGER) - CAST(r.message_ts AS INTEGER) AS secs_to_next,
  substr(replace(cur.text, char(10), ' '), -120) AS text_tail
FROM report r
LEFT JOIN messages cur ON cur.ts = r.message_ts AND cur.channel_id = r.channel_id
LEFT JOIN messages next ON next.ts = r.next_ts AND next.channel_id = r.channel_id
LEFT JOIN users u ON u.id = next.user_id;

.print '== rows by state and source'
SELECT state, source, COUNT(*) AS n,
  datetime(MIN(reported_at), 'unixepoch', 'localtime') AS first,
  datetime(MAX(reported_at), 'unixepoch', 'localtime') AS last
FROM agent_state_reports GROUP BY state, source ORDER BY state, source;

.print ''
.print '== blocked, but the agent continued without a human'
SELECT reported, channel_id, thread_ts, message_ts, judge_reply, secs_to_next, text_tail
FROM report_next
WHERE next_is_bot = 1
  AND id IN (SELECT MIN(id) FROM agent_state_reports WHERE state = 'blocked'
             GROUP BY channel_id, thread_ts, message_ts)
ORDER BY id;

.print ''
.print '== working, then silence until a human posted (1h or more)'
SELECT reported, channel_id, thread_ts, message_ts, source,
  COALESCE(secs_to_next, 'none yet') AS secs_to_next, text_tail
FROM report_next
WHERE state = 'working'
  AND id IN (SELECT MAX(id) FROM agent_state_reports
             GROUP BY channel_id, thread_ts, message_ts)
  AND ((next_is_bot = 0 AND secs_to_next >= 3600)
    OR (next_ts IS NULL AND reported_at < unixepoch() - 3600))
ORDER BY id;

.print ''
.print '== judge errors, newest 20'
SELECT datetime(reported_at, 'unixepoch', 'localtime') AS reported,
  channel_id, thread_ts, message_ts, judge_model, error
FROM agent_state_reports
WHERE source = 'judge_error'
ORDER BY id DESC LIMIT 20;
SQL
