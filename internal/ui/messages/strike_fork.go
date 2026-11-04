package messages

// replaceStrikethroughs rewrites `~X~` runs through render when the
// tildes sit on Slack's strike boundaries (replaceDelimitedRuns in
// emphasis_fork.go); tildes that don't pair up (the prose "~45%" meaning
// approximately) stay literal. The old regex `~([^~\n]+)~` paired any
// two same-line tildes, crossing out everything between two
// "approximately" markers.
func replaceStrikethroughs(text string, render func(inner string) string) string {
	return replaceDelimitedRuns(text, '~', render)
}

// markStrike marks a struck run for drawSpans (stylespans_fork.go), which
// strikes it once what sits inside it is drawn.
func markStrike(inner string) string {
	return markSpan(spanStrike, inner)
}
