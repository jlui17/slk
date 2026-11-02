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
	sub, m := matchWorkspacePath(rawURL, profilePathRe)
	if m == nil {
		return "", "", false
	}
	return sub, m[1], true
}

// matchWorkspacePath matches the path of an https link into a
// workspace, https://<subdomain>.slack.com/..., against re: the
// subdomain and re's submatches, nil for any other link.
func matchWorkspacePath(rawURL string, re *regexp.Regexp) (subdomain string, match []string) {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return "", nil
	}
	sub, found := strings.CutSuffix(u.Host, ".slack.com")
	if !found || sub == "" {
		return "", nil
	}
	return sub, re.FindStringSubmatch(u.Path)
}
