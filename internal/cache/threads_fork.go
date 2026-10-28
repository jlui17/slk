package cache

// The unary + on the two newest-activity subqueries' channel_id keeps
// SQLite off idx_messages_channel: through it, each subscription reads
// every cached message in its channel, which with a thousand
// subscriptions in busy channels ran for minutes per call. Without it
// the planner seeks the thread_ts and ts indexes for the OR.
const listSubscribedThreadsQuery = `
SELECT
    s.channel_id,
    s.thread_ts,
    COALESCE(c.name, ''),
    COALESCE(c.type, ''),
    s.last_read,
    s.latest_reply,
    COALESCE((SELECT user_id FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), ''),
    COALESCE((SELECT text FROM messages
              WHERE channel_id = s.channel_id AND ts = s.thread_ts AND is_deleted = 0), ''),
    (SELECT COUNT(*) FROM messages
     WHERE channel_id = s.channel_id AND thread_ts = s.thread_ts
       AND ts != s.thread_ts AND is_deleted = 0)
        AS reply_count,
    COALESCE(
        (SELECT MAX(ts) FROM messages
         WHERE +channel_id = s.channel_id AND (thread_ts = s.thread_ts OR ts = s.thread_ts)
           AND is_deleted = 0),
        ''
    ) AS cached_max_ts,
    COALESCE(
        (SELECT user_id FROM messages
         WHERE +channel_id = s.channel_id AND (thread_ts = s.thread_ts OR ts = s.thread_ts)
           AND is_deleted = 0
         ORDER BY ts DESC LIMIT 1),
        ''
    ) AS last_reply_by
FROM thread_subscriptions s
LEFT JOIN channels c ON c.id = s.channel_id
WHERE s.workspace_id = ? AND s.active = 1
`
