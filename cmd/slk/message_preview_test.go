package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// A deleted message's tombstone row still carries its text; the
// preview cache tier must report it as found-but-empty, never serve
// the text or fall through to a doomed network fetch.
func TestCachedMessagePreview(t *testing.T) {
	db := newCacheForTest(t)
	if err := db.UpsertMessage(cache.Message{
		ChannelID: "C1", TS: "1700000000.000100", UserID: "U1", Text: "hello",
	}); err != nil {
		t.Fatalf("UpsertMessage: %v", err)
	}

	userID, text, found := cachedMessagePreview(db, "C1", "1700000000.000100")
	if !found || userID != "U1" || text != "hello" {
		t.Errorf("live message = (%q, %q, %v), want (U1, hello, true)", userID, text, found)
	}

	if _, _, found := cachedMessagePreview(db, "C1", "1700000000.999999"); found {
		t.Error("missing message reported as found")
	}

	if err := db.DeleteMessage("C1", "1700000000.000100"); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	userID, text, found = cachedMessagePreview(db, "C1", "1700000000.000100")
	if !found || userID != "" || text != "" {
		t.Errorf("tombstone = (%q, %q, %v), want found with no sender/text", userID, text, found)
	}
}
