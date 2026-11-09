package slackurl

import "strings"

// ThreadPermalink builds the link to a thread's root message in the form
// Slack's "Copy link" gives a thread reply, which Parse reads back as
// that thread with its root as the target message. Empty when subdomain,
// channelID or threadTS is.
func ThreadPermalink(subdomain, channelID, threadTS string) string {
	if subdomain == "" || channelID == "" || threadTS == "" {
		return ""
	}
	return "https://" + subdomain + ".slack.com/archives/" + channelID +
		"/p" + strings.ReplaceAll(threadTS, ".", "") +
		"?thread_ts=" + threadTS + "&cid=" + channelID
}
