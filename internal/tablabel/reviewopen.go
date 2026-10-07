package tablabel

// ReviewOpenHints are the hint lines a !review-open thread's request adds
// after the user's own: its root is a form that names nothing, and the line
// after it is the annotated message (see internal/ui/agentthread_reviewopen.go).
// They go after the user's hints because the first one overrides a hint that
// takes the id from the root. Measured on 8 real threads with the user's
// hints, they put the annotated message's PR number on the id line in 18 of
// 18 runs, and no id in the 30 runs of threads whose annotated message names
// none.
var ReviewOpenHints = []string{
	"this thread's root starts with !review-open: it reviews an annotation, and the line right " +
		"after the root is the annotated message, a person's complaint about what the agent did. " +
		"For this thread the hint about the id in the root does not apply: the id is the PR or " +
		"task number the annotated message names, as in 'pr 1231' or a bare '1808 is already " +
		"merged', written with a # (#1231, #1808), or a task id as written (colony-123); when the " +
		"annotated message names no number, answer none",
	"for this thread the label names what the complaint says went wrong, in 1 to 3 words (for " +
		"example wrong branch, skipped tests); never the words review, annotation, judge, open or claude",
}
