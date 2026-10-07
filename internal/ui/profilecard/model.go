// Package profilecard is the card a click on an @mention opens, as
// Slack opens a profile popup: who the person is, whether they are
// around, their local time and their status.
package profilecard

import (
	"context"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/cellwidth"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/peerstatus"
	"github.com/gammons/slk/internal/ui/styles"
)

// Profile is what the card shows. An empty field is left off the card.
type Profile struct {
	UserID      string
	RealName    string
	DisplayName string
	Title       string
	// Presence is "active" or "away"; "" while unknown.
	Presence string
	// TZ is the IANA zone name; TZOffset (seconds east of UTC) stands
	// in when the zone can't be loaded.
	TZ       string
	TZOffset int
	Status   peerstatus.Status
}

// Service fetches a user's full profile in a workspace.
type Service interface {
	Profile(ctx context.Context, teamID, userID string) (Profile, error)
}

// Model is the card. The App decides when it is shown.
type Model struct {
	p   Profile
	now func() time.Time
	// loc is the user's zone, loaded from p.TZ; nil when p.TZ is empty.
	// zoneKnown is false when the name didn't load and loc is the bare
	// offset, whose abbreviation the card can't show.
	loc       *time.Location
	zoneKnown bool
}

// Open shows p, what is known of the user so far, with the local time
// read from now.
func (m *Model) Open(p Profile, now func() time.Time) {
	m.now = now
	m.set(p)
}

// Profile is the profile the card shows.
func (m *Model) Profile() Profile { return m.p }

// Fill takes a fetched profile. It replaces what the card showed, but a
// presence the fetch came back without is kept, and so are DND and the
// huddle, which users.info doesn't carry.
func (m *Model) Fill(fetched Profile) {
	if fetched.Presence == "" {
		fetched.Presence = m.p.Presence
	}
	s := fetched.Status
	fetched.Status = m.p.Status.WithStatus(s.Emoji, s.Text, s.Expires)
	m.set(fetched)
}

func (m *Model) set(p Profile) {
	m.p, m.loc, m.zoneKnown = p, nil, false
	if p.TZ == "" {
		return
	}
	loc, err := time.LoadLocation(p.TZ)
	if err != nil {
		m.loc = time.FixedZone("", p.TZOffset)
		return
	}
	m.loc, m.zoneKnown = loc, true
}

// Footer is the card's last row: the keys it takes.
const Footer = "m message · o open in Slack · esc"

// title is the card's name: the real name, else the display name.
func (m *Model) title() string {
	for _, s := range []string{m.p.RealName, m.p.DisplayName, m.p.UserID} {
		if s != "" {
			return s
		}
	}
	return ""
}

// Lines are the card's rows between its title and its footer, as plain
// text: the handle and title, presence and local time, status and DND.
// A row with nothing to show is left out.
func (m *Model) Lines() []string {
	var lines []string
	for _, r := range m.rows() {
		line := r.text
		if r.dot != "" {
			line = r.dot + " " + line
		}
		lines = append(lines, line)
	}
	return lines
}

// row is one row of the card. dot, drawn before the text, is the
// presence dot: the one cell the card colors by meaning, always followed
// by the word.
type row struct {
	dot  string
	text string
}

func (m *Model) rows() []row {
	var rows []row
	var who []string
	if m.p.DisplayName != "" {
		who = append(who, "@"+m.p.DisplayName)
	}
	if m.p.Title != "" {
		who = append(who, m.p.Title)
	}
	if len(who) > 0 {
		rows = append(rows, row{text: strings.Join(who, " · ")})
	}
	var here []string
	dot := ""
	switch m.p.Presence {
	case "active":
		dot, here = "●", append(here, "Active")
	case "away":
		dot, here = "○", append(here, "Away")
	}
	if t := m.localTime(); t != "" {
		here = append(here, t)
	}
	if len(here) > 0 {
		rows = append(rows, row{dot: dot, text: strings.Join(here, " · ")})
	}
	// Slack's popup shows the huddle elsewhere; the card shows the
	// status and DND.
	if s := m.p.Status.WithHuddle("", time.Time{}).Summary(m.now(), messages.TimestampFormat()); s != "" {
		rows = append(rows, row{text: s})
	}
	return rows
}

// The presence dot's colors, blue for active and orange for away under
// every theme, so the pair never depends on a theme's palette.
var (
	activeBlue = lipgloss.Color("#3B82F6")
	awayOrange = lipgloss.Color("#E07B24")
)

func presenceColor(presence string) lipgloss.Style {
	s := lipgloss.NewStyle().Background(styles.Background)
	if presence == "active" {
		return s.Foreground(activeBlue)
	}
	return s.Foreground(awayOrange)
}

// localTime is the time where the user is, "2:41 PM local (PDT)" in the
// configured timestamp format, or "" when their zone is unknown.
func (m *Model) localTime() string {
	if m.loc == nil {
		return ""
	}
	t := m.now().In(m.loc)
	s := t.Format(messages.TimestampFormat()) + " local"
	if m.zoneKnown {
		s += " (" + t.Format("MST") + ")"
	}
	return s
}

// BoxSize is the card's outer size, for the click router.
func (m *Model) BoxSize(termWidth, termHeight int) (int, int) {
	box := m.box(termWidth)
	return lipgloss.Width(box), lipgloss.Height(box)
}

// ViewOverlay draws the card centered over background.
func (m *Model) ViewOverlay(termWidth, termHeight int, background string) string {
	return overlay.DimmedOverlay(termWidth, termHeight, background, m.box(termWidth), 0.5)
}

// box is the card: the name in its top border, as Slack heads the
// popup with it, then the rows, a blank row and the footer.
func (m *Model) box(termWidth int) string {
	title, rows := " "+m.title()+" ", m.rows()
	inner := max(lipgloss.Width(Footer), lipgloss.Width(title))
	for _, l := range m.Lines() {
		inner = max(inner, lipgloss.Width(l))
	}
	inner = max(min(inner, termWidth-4), 1)

	bg := lipgloss.NewStyle().Background(styles.Background)
	text := bg.Foreground(styles.TextPrimary)
	var lines []string
	for _, r := range rows {
		if r.dot == "" {
			lines = append(lines, text.Render(cellwidth.Cut(r.text, inner)))
			continue
		}
		lines = append(lines, presenceColor(m.p.Presence).Render(r.dot)+text.Render(" "+cellwidth.Cut(r.text, max(inner-2, 0))))
	}
	lines = append(lines, "", bg.Foreground(styles.TextMuted).Render(cellwidth.Cut(Footer, inner)))
	content := messages.ReapplyBgAfterResets(strings.Join(lines, "\n"), messages.BgANSI()+messages.FgANSI())
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(styles.Background).
		Background(styles.Background).
		Padding(0, 1).
		Width(inner + 4). // border and one column of padding on each side
		Render(content)

	// The name sits in the top border one cell in from the corner, as
	// withCopyLabel splices its label in (messages/codeblocks_fork.go).
	top, rest, _ := strings.Cut(box, "\n")
	width := lipgloss.Width(top)
	title = cellwidth.Cut(title, width-4)
	name := bg.Foreground(styles.Primary).Bold(true).Render(title)
	return ansi.Cut(top, 0, 2) + name + ansi.Cut(top, 2+lipgloss.Width(title), width) + "\n" + rest
}
