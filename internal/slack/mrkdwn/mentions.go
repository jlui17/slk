package mrkdwn

import "regexp"

// reUserAnyForm additionally accepts the labeled wire form <@U1|label>,
// which Slack uses in search snippets and can surface in message text;
// the token grammar in tokens.go stays bare-form-only because the styled
// renderer never sees labels.
var reUserAnyForm = regexp.MustCompile(`<@([UW][A-Z0-9]+)[|>]`)

// MentionedUserIDs returns the user ID of every <@U…> mention in a raw
// mrkdwn string, labeled or bare, in order of appearance.
func MentionedUserIDs(input string) []string {
	var ids []string
	for _, m := range reUserAnyForm.FindAllStringSubmatch(input, -1) {
		ids = append(ids, m[1])
	}
	return ids
}
