package messages

import "github.com/gammons/slk/internal/slackurl"

// profileLink wraps a drawn user mention in an OSC 8 link to the user's
// profile in the workspace at domain, as Slack links a mention: a click
// opens the profile card (routeLink), and the terminal's own link
// handling opens the profile in the browser. Without a domain there is
// no URL a browser could open, so the mention stays plain.
func profileLink(domain, userID, drawn string) string {
	if domain == "" {
		return drawn
	}
	return osc8Hyperlink(slackurl.ProfileURL(domain, userID), drawn)
}

// SetWorkspaceDomain sets the subdomain user mentions link into.
func (m *Model) SetWorkspaceDomain(domain string) {
	if m.workspaceDomain == domain {
		return
	}
	m.workspaceDomain = domain
	m.InvalidateCache()
}
