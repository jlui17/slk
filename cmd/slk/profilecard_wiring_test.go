package main

import (
	"reflect"
	"testing"
	"time"

	"github.com/slack-go/slack"

	"github.com/gammons/slk/internal/ui/peerstatus"
	"github.com/gammons/slk/internal/ui/profilecard"
)

func TestProfileFromUser(t *testing.T) {
	u := &slack.User{
		ID: "U2", Name: "dkim", RealName: "Dana Kim", TZ: "America/Los_Angeles", TZOffset: -25200,
		Profile: slack.UserProfile{
			DisplayName: "dana", Title: "Staff Engineer",
			StatusEmoji: ":palm_tree:", StatusText: "Out until Monday", StatusExpiration: 1791500000,
		},
	}
	want := profilecard.Profile{
		UserID: "U2", RealName: "Dana Kim", DisplayName: "dana", Title: "Staff Engineer",
		TZ: "America/Los_Angeles", TZOffset: -25200,
		Status: peerstatus.Status{Emoji: ":palm_tree:", Text: "Out until Monday", Expires: time.Unix(1791500000, 0)},
	}
	if got := profileFromUser(u); !reflect.DeepEqual(got, want) {
		t.Errorf("profileFromUser = %+v\nwant %+v", got, want)
	}

	// No display name: the real name stands in (the profile's, when the
	// user has none), and the username when there is no real name either.
	u = &slack.User{ID: "U3", Name: "sam", Profile: slack.UserProfile{RealName: "Sam Lee"}}
	if got := profileFromUser(u); got.DisplayName != "Sam Lee" || got.RealName != "Sam Lee" || !got.Status.Expires.IsZero() {
		t.Errorf("profileFromUser without a display name = %+v", got)
	}
	u = &slack.User{ID: "U4", Name: "kim"}
	if got := profileFromUser(u); got.DisplayName != "kim" {
		t.Errorf("profileFromUser with only a username: display name %q, want kim", got.DisplayName)
	}
}
