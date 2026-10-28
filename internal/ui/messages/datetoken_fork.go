package messages

import (
	"fmt"
	"regexp"
	"time"
)

// slackDateAgo renders Slack's {ago} token: the time between t and now
// in its largest whole unit ("3 minutes ago", "4 hours ago", "2 days
// ago", per Slack's formatting docs), or "in 3 minutes" for a future t.
// Under a minute either way reads "just now"; Slack's docs don't show
// that case. The text is fixed when the message renders, not ticking.
func slackDateAgo(t, now time.Time) string {
	d := now.Sub(t)
	future := d < 0
	if future {
		d = -d
	}
	days := int(d / (24 * time.Hour))
	var n int
	var unit string
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		n, unit = int(d/time.Minute), "minute"
	case d < 24*time.Hour:
		n, unit = int(d/time.Hour), "hour"
	case days < 30:
		n, unit = days, "day"
	case days < 365:
		n, unit = days/30, "month"
	default:
		n, unit = days/365, "year"
	}
	if n != 1 {
		unit += "s"
	}
	if future {
		return fmt.Sprintf("in %d %s", n, unit)
	}
	return fmt.Sprintf("%d %s ago", n, unit)
}

var slackDateAnyTokenRe = regexp.MustCompile(`\{[a-z_]+\}`)

// hasUnknownSlackDateToken reports whether format carries a {token}
// slk cannot expand; the whole token then draws its fallback text
// rather than raw braces.
func hasUnknownSlackDateToken(format string) bool {
	for _, tok := range slackDateAnyTokenRe.FindAllString(format, -1) {
		if slackDateFormatRe.FindString(tok) != tok {
			return true
		}
	}
	return false
}
