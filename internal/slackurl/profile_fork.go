package slackurl

import (
	"net/url"
	"regexp"
	"strings"
)

// ProfileURL is a user's profile page in the workspace at
// <subdomain>.slack.com, the link Slack's web client gives a mention.
func ProfileURL(subdomain, userID string) string {
	return "https://" + subdomain + ".slack.com/team/" + userID
}

var profilePathRe = regexp.MustCompile(`^/team/([UW][A-Z0-9]+)/?$`)

// ParseProfile reads a ProfileURL back: ok is false for anything else.
func ParseProfile(rawURL string) (subdomain, userID string, ok bool) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return "", "", false
	}
	sub, found := strings.CutSuffix(u.Host, ".slack.com")
	if !found || sub == "" {
		return "", "", false
	}
	m := profilePathRe.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", false
	}
	return sub, m[1], true
}
