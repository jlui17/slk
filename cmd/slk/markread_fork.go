package main

// threadMarkReadState decides what a thread_marked watermark means for
// read state. Equality means caught up: a genuine mark-as-unread sets
// last_read strictly before the message being marked, so last_read can
// only reach the newest activity by reading to the end. A watermark past
// everything known (or nothing known at all) is undecidable — the local
// newest-activity view is stale by definition, and a read-to-end cannot
// be told apart from a mark-unread at an uncached reply, which is a
// deliberate user signal that must not be guessed away. Callers persist
// the facts and skip the UI dispatch; the next getView reconcile settles
// it. String comparison is valid: Slack ts are fixed-width secs.micros.
func threadMarkReadState(lastRead, newestActivity string) (read, known bool) {
	if newestActivity == "" || lastRead > newestActivity {
		return false, false
	}
	return lastRead == newestActivity, true
}
