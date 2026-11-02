package slackurl

import (
	"regexp"

	"github.com/gammons/slk/internal/ids"
)

var channelPathRe = regexp.MustCompile(`^/archives/([A-Z0-9]+)/?$`)

// ParseChannel decodes a link to a conversation itself,
// https://<subdomain>.slack.com/archives/<CHANNEL>, as a bookmark to a
// channel has it. ok is false for anything else, a message permalink
// included.
func ParseChannel(rawURL string) (subdomain string, channelID ids.ChannelID, ok bool) {
	sub, m := matchWorkspacePath(rawURL, channelPathRe)
	if m == nil {
		return "", "", false
	}
	return sub, ids.ChannelID(m[1]), true
}
