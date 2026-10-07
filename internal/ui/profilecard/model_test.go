package profilecard

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/peerstatus"
)

// at is 2:41 PM in Los Angeles, daylight time.
func at() time.Time { return time.Date(2026, 10, 7, 21, 41, 0, 0, time.UTC) }

func dana() Profile {
	return Profile{
		UserID: "U2", RealName: "Dana Kim", DisplayName: "dana", Title: "Staff Engineer",
		Presence: "active", TZ: "America/Los_Angeles",
		Status: peerstatus.Status{Emoji: ":palm_tree:", Text: "Out until Monday"},
	}
}

func TestCardShowsEveryPart(t *testing.T) {
	var m Model
	m.Open(dana(), at)
	want := []string{"@dana · Staff Engineer", "● Active · 2:41 PM local (PDT)", "🌴 Out until Monday"}
	if got := m.Lines(); !reflect.DeepEqual(got, want) {
		t.Errorf("Lines() = %q, want %q", got, want)
	}
	box := ansi.Strip(m.box(120))
	rows := strings.Split(box, "\n")
	if !strings.HasPrefix(rows[0], "╭─ Dana Kim ─") || !strings.HasSuffix(rows[0], "╮") {
		t.Errorf("top border %q, want the name in it", rows[0])
	}
	if !strings.Contains(rows[len(rows)-2], Footer) {
		t.Errorf("last row inside the box %q, want the footer", rows[len(rows)-2])
	}
	w, h := m.BoxSize(120, 40)
	for _, r := range rows {
		if got := ansi.StringWidth(r); got != w {
			t.Errorf("row %q is %d wide, box %d", r, got, w)
		}
	}
	if h != len(rows) {
		t.Errorf("BoxSize height %d, box has %d rows", h, len(rows))
	}
}

func TestCardUsesTheTimestampFormat(t *testing.T) {
	messages.SetTimestampFormat("15:04")
	t.Cleanup(func() { messages.SetTimestampFormat("") })
	var m Model
	m.Open(dana(), at)
	if got := m.Lines()[1]; got != "● Active · 14:41 local (PDT)" {
		t.Errorf("presence row %q, want the 24-hour time", got)
	}
}

func TestCardOmitsWhatIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		name  string
		edit  func(*Profile)
		title string
		want  []string
	}{
		{"no title", func(p *Profile) { p.Title = "" }, "Dana Kim",
			[]string{"@dana", "● Active · 2:41 PM local (PDT)", "🌴 Out until Monday"}},
		{"away, no zone", func(p *Profile) { p.Presence, p.TZ = "away", "" }, "Dana Kim",
			[]string{"@dana · Staff Engineer", "○ Away", "🌴 Out until Monday"}},
		{"presence unknown", func(p *Profile) { p.Presence = "" }, "Dana Kim",
			[]string{"@dana · Staff Engineer", "2:41 PM local (PDT)", "🌴 Out until Monday"}},
		{"status expired", func(p *Profile) { p.Status.Expires = at().Add(-time.Minute) }, "Dana Kim",
			[]string{"@dana · Staff Engineer", "● Active · 2:41 PM local (PDT)"}},
		{"only a display name", func(p *Profile) { *p = Profile{UserID: "U2", DisplayName: "dana"} }, "dana",
			[]string{"@dana"}},
		{"nothing known", func(p *Profile) { *p = Profile{UserID: "U2"} }, "U2", nil},
		{"zone slk can't load", func(p *Profile) { p.TZ, p.TZOffset = "Nowhere/Atlantis", -7*3600 }, "Dana Kim",
			[]string{"@dana · Staff Engineer", "● Active · 2:41 PM local", "🌴 Out until Monday"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := dana()
			tc.edit(&p)
			var m Model
			m.Open(p, at)
			if got := m.Lines(); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Lines() = %q, want %q", got, tc.want)
			}
			if got := m.title(); got != tc.title {
				t.Errorf("title() = %q, want %q", got, tc.title)
			}
		})
	}
}

// DND shows as Slack's popup shows it, after the status, with its end
// in the configured format; a huddle doesn't show.
func TestCardShowsDNDButNotTheHuddle(t *testing.T) {
	end := at().Add(90 * time.Minute)
	p := dana()
	p.Status = p.Status.WithDND(true, end).WithHuddle(peerstatus.HuddleActive, time.Time{})
	var m Model
	m.Open(p, at)
	want := "🌴 Out until Monday · ⊘ Do not disturb until " + end.Local().Format(messages.TimestampFormat())
	if got := m.Lines()[2]; got != want {
		t.Errorf("status row %q, want %q", got, want)
	}
}

func TestFillKeepsWhatTheFetchLacks(t *testing.T) {
	var m Model
	known := Profile{UserID: "U2", DisplayName: "dana", Presence: "away", Status: peerstatus.Status{}.WithDND(true, time.Time{})}
	m.Open(known, at)
	fetched := dana()
	fetched.Presence = ""
	m.Fill(fetched)
	p := m.Profile()
	if p.Presence != "away" || p.RealName != "Dana Kim" || p.Status.Text != "Out until Monday" || !p.Status.DND {
		t.Errorf("after Fill: %+v", p)
	}
	if got := m.Lines()[1]; got != "○ Away · 2:41 PM local (PDT)" {
		t.Errorf("after Fill: presence row %q, want the fetched zone's time", got)
	}
}
