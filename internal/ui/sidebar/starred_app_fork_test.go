package sidebar

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// A starred app DM sits under Starred as an app row, not in Apps or
// Direct Messages.
func TestStarredAppDM_RendersUnderStarredAsAnApp(t *testing.T) {
	items := []ChannelItem{
		{ID: "D9", Name: "Colony", Type: "app", Section: "ST"},
		{ID: "D2", Name: "alice", Type: "dm"},
	}
	m := New(items)
	m.SetSectionsProvider(&fakeProvider{ready: true, sections: []SectionMeta{
		{ID: "ST", Type: "stars"},
		{ID: "DM", Name: "Direct Messages", Type: "direct_messages"},
		{ID: "APPS", Name: "Apps", Type: "recent_apps"},
	}})
	out := ansi.Strip(m.View(30, 40))
	starred, dms := strings.Index(out, "Starred"), strings.Index(out, "Direct Messages")
	colony := strings.Index(out, "▣ Colony")
	if starred < 0 || colony < 0 || dms < 0 || !(starred < colony && colony < dms) {
		t.Errorf("want ▣ Colony between Starred and Direct Messages:\n%s", out)
	}
}
