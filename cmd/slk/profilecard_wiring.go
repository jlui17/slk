package main

import (
	"context"
	"sync"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gammons/slk/internal/ui/peerstatus"
	"github.com/gammons/slk/internal/ui/profilecard"
)

// profileService is the profile card's profilecard.Service, routed to
// the workspace the card was opened in: users.info for the profile and
// users.getPresence for whether the user is around, sent together.
type profileService struct{ router *workspaceRouter }

func (s profileService) Profile(ctx context.Context, teamID, userID string) (profilecard.Profile, error) {
	c, err := s.router.Client(teamID)
	if err != nil {
		return profilecard.Profile{}, err
	}
	var presence *slack.UserPresence
	var presenceErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		presence, presenceErr = c.GetUserPresence(ctx, userID)
	}()
	u, err := c.GetUserProfileContext(ctx, userID)
	wg.Wait()
	if err != nil {
		return profilecard.Profile{}, err
	}
	p := profileFromUser(u)
	if presenceErr != nil {
		debuglog.General("profile card: presence of %s: %v", userID, presenceErr)
	} else {
		p.Presence = presence.Presence
	}
	return p, nil
}

// profileFromUser is the card's profile from a users.info user. The
// display name falls back to the real name, then the username, as every
// other name resolver in slk does.
func profileFromUser(u *slack.User) profilecard.Profile {
	realName := u.RealName
	if realName == "" {
		realName = u.Profile.RealName
	}
	display := u.Profile.DisplayName
	if display == "" {
		display = realName
	}
	if display == "" {
		display = u.Name
	}
	return profilecard.Profile{
		UserID:      u.ID,
		RealName:    realName,
		DisplayName: display,
		Title:       u.Profile.Title,
		TZ:          u.TZ,
		TZOffset:    u.TZOffset,
		Status: peerstatus.Status{}.
			WithStatus(u.Profile.StatusEmoji, u.Profile.StatusText, statusExpiry(int64(u.Profile.StatusExpiration))),
	}
}
