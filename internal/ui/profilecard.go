package ui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/slackurl"
	"github.com/gammons/slk/internal/ui/profilecard"
)

// profileCardState is the card a click on an @mention opens. It shows
// while the App is in ModeProfileCard, so anything that sets another
// mode (a workspace switch, the quit prompt) puts it away.
type profileCardState struct {
	svc    profilecard.Service
	model  profilecard.Model
	teamID string
}

type profileLoadedMsg struct {
	teamID, userID string
	profile        profilecard.Profile
	err            error
}

// SetProfileService wires the card's fetch of the full profile.
func (a *App) SetProfileService(s profilecard.Service) { a.profileCard.svc = s }

// setWorkspaceDomain tells every pane the active workspace's subdomain,
// which user mentions link into.
func (a *App) setWorkspaceDomain() {
	domain := a.activeWorkspaceDomain()
	for _, m := range a.allWinModels() {
		m.SetWorkspaceDomain(domain)
	}
	a.threadPanel.SetWorkspaceDomain(domain)
}

// routeProfileLink opens the card for a link to a user's profile in the
// active workspace: what a click on a mention opens. ok is false for any
// other link, and for O, which opens links in herdr tabs.
func (a *App) routeProfileLink(rawURL string, inHerdrTab bool) (tea.Cmd, bool) {
	sub, userID, ok := slackurl.ParseProfile(rawURL)
	if !ok || inHerdrTab || sub != a.activeWorkspaceDomain() {
		return nil, false
	}
	return a.openProfileCard(userID), true
}

// openProfileCard shows what slk knows of the user, and fetches the rest.
func (a *App) openProfileCard(userID string) tea.Cmd {
	s := &a.profileCard
	known := profilecard.Profile{
		UserID:   userID,
		Presence: a.sidebar.PresenceOf(userID),
		Status:   a.presence.peers[userID],
	}
	known.DisplayName, _ = a.userNames.Get(userID)
	s.teamID = a.activeTeamID
	s.model.Open(known, a.now)
	a.SetMode(ModeProfileCard)
	if s.svc == nil {
		return nil
	}
	svc, teamID := s.svc, s.teamID
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		p, err := svc.Profile(ctx, teamID, userID)
		return profileLoadedMsg{teamID: teamID, userID: userID, profile: p, err: err}
	}
}

// reduceProfileCard fills the card with the fetched profile, when the
// card still shows that user.
var reduceProfileCard reducerFunc = func(a *App, msg tea.Msg) (tea.Cmd, bool) {
	m, ok := msg.(profileLoadedMsg)
	if !ok {
		return nil, false
	}
	s := &a.profileCard
	if a.mode != ModeProfileCard || m.teamID != s.teamID || m.userID != s.model.Profile().UserID {
		return nil, true
	}
	if m.err != nil {
		debuglog.General("profile card: %s: %v", m.userID, m.err)
		return func() tea.Msg { return ToastMsg{Text: "Failed to load profile"} }, true
	}
	s.model.Fill(m.profile)
	return nil, true
}

// handleProfileCardMode: m messages the user, o opens their profile in
// the browser, esc closes. The card holds every other key.
func handleProfileCardMode(a *App, msg tea.KeyMsg) tea.Cmd {
	userID := a.profileCard.model.Profile().UserID
	switch msg.String() {
	case "esc", "q":
		a.SetMode(ModeNormal)
	case "m":
		a.SetMode(ModeNormal)
		return a.openConversationCmd([]string{userID})
	case "o":
		a.SetMode(ModeNormal)
		return a.browserOpener(slackurl.ProfileURL(a.activeWorkspaceDomain(), userID))
	}
	return nil
}

// profileCardOverlay draws the card over screen while it is open.
func (a *App) profileCardOverlay(screen string) string {
	if a.mode != ModeProfileCard {
		return screen
	}
	return a.profileCard.model.ViewOverlay(a.width, a.height, screen)
}
