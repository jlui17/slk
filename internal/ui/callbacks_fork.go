package ui

// ReloadFunc forces every workspace's websocket to reconnect now (the
// manual reload, slk's cmd+r analog): pending backoff waits are
// skipped and the reconnect catch-up dedupe gates are reset so the
// catch-up pass runs even right after a natural reconnect.
type ReloadFunc func()
