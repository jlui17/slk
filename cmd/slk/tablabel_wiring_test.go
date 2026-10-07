package main

import (
	"slices"
	"testing"

	"github.com/gammons/slk/internal/tablabel"
)

func TestRelabelHints(t *testing.T) {
	user := make([]string, 2, 8) // spare capacity an append would write into
	user[0], user[1] = "hint one", "hint two"

	if got := relabelHints(user, false); !slices.Equal(got, user) {
		t.Errorf("other thread's hints = %q, want the user's alone", got)
	}

	got := relabelHints(user, true)
	if want := slices.Concat(user, tablabel.ReviewOpenHints); !slices.Equal(got, want) {
		t.Errorf("review-open hints = %q, want the user's then tablabel.ReviewOpenHints", got)
	}
	if extra := user[:3][2]; extra != "" {
		t.Errorf("review-open hints wrote %q into the user's backing array", extra)
	}
}
