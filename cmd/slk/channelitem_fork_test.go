package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/slack-go/slack"
)

// Both upsertChannelInDB call sites carry membership by construction
// (the boot joined-conversation list; channel_joined-family WS
// events), while the wire payloads at those sites ship is_member
// false. A boot must therefore not clobber the is_member=1 row
// hydrateFirstSight wrote — the cache-first sidebar paint filters on
// it and goes dark for the whole workspace when a boot zeroes it.
func TestUpsertChannelInDB_PreservesMembershipDespiteFalseWireField(t *testing.T) {
	db, err := cache.New(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.UpsertWorkspace(cache.Workspace{ID: "T1", Name: "t1"}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertChannel(cache.Channel{
		ID:          "C123",
		WorkspaceID: "T1",
		Name:        "general",
		Type:        "channel",
		IsMember:    true,
	}); err != nil {
		t.Fatal(err)
	}

	wire := slack.Channel{}
	wire.ID = "C123"
	wire.Name = "general"
	wire.IsMember = false
	upsertChannelInDB(db, wire, "channel", "T1")

	got, err := db.GetChannel("C123")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMember {
		t.Fatal("boot upsert zeroed is_member; cache-first paint filters on it and can never fire")
	}
}
