package herdr

import (
	"path/filepath"
	"testing"
	"time"
)

func labelCacheReporter(t *testing.T, currentLabel string) (*Reporter, *recorder) {
	t.Helper()
	sock := filepath.Join(t.TempDir(), "herdr.sock")
	_, rec := startServer(t, "unix", sock)
	rec.setTabLabel(currentLabel)
	return newReporter("unix", sock, "w1:p1", "w1:t1"), rec
}

// A herdr restart keeps tab labels but wipes pane-metadata tokens, so a
// label a previous slk run set (a model-generated one especially, which no
// rerun reproduces) reads as unowned. The label cache is slk's own durable
// record of what it last set: a match re-claims the tab.
func TestNameTabReclaimsViaLabelCache(t *testing.T) {
	r, rec := labelCacheReporter(t, "Fix flow viewer stale runs")
	r.SetTabLabelCache(
		func() (string, bool) { return "Fix flow viewer stale runs", true },
		func(string) error { return nil },
	)

	r.NameTab("the flow viewer renders sta…")
	r.Close(time.Second)

	if got := rec.getTabLabel(); got != "the flow viewer renders sta…" {
		t.Errorf("tab label = %q, want the rename to land", got)
	}
	if got := rec.getTokens()[tabLabelToken]; got != "the flow viewer renders sta…" {
		t.Errorf("ownership token = %q, want re-recorded", got)
	}
}

func TestNameTabRefusesOnLabelCacheMiss(t *testing.T) {
	r, rec := labelCacheReporter(t, "my precious tab")
	r.SetTabLabelCache(
		func() (string, bool) { return "", false },
		func(string) error { return nil },
	)

	r.NameTab("fix retries")
	r.Close(time.Second)

	if got := rec.getTabLabel(); got != "my precious tab" {
		t.Errorf("tab label = %q, user label must stand", got)
	}
}

func TestNameTabRefusesOnLabelCacheMismatch(t *testing.T) {
	r, rec := labelCacheReporter(t, "my precious tab")
	r.SetTabLabelCache(
		func() (string, bool) { return "some other slk label", true },
		func(string) error { return nil },
	)

	r.NameTab("fix retries")
	r.Close(time.Second)

	if got := rec.getTabLabel(); got != "my precious tab" {
		t.Errorf("tab label = %q, user label must stand", got)
	}
}

// Every label NameTab sets (rename or claim of an already-matching label)
// lands in the cache, so the next process can prove ownership after herdr
// forgets the token.
func TestNameTabSavesLabelCache(t *testing.T) {
	r, _ := labelCacheReporter(t, "3")
	var saved []string
	r.SetTabLabelCache(
		func() (string, bool) { return "", false },
		func(label string) error { saved = append(saved, label); return nil },
	)

	r.NameTab("fix retries")
	r.Close(time.Second)

	if len(saved) != 1 || saved[0] != "fix retries" {
		t.Errorf("saved labels = %v, want [fix retries]", saved)
	}
}

func TestNameTabClaimOfEqualLabelSavesCache(t *testing.T) {
	r, _ := labelCacheReporter(t, "fix retries")
	var saved []string
	r.SetTabLabelCache(
		func() (string, bool) { return "", false },
		func(label string) error { saved = append(saved, label); return nil },
	)

	r.NameTab("fix retries")
	r.Close(time.Second)

	if len(saved) != 1 || saved[0] != "fix retries" {
		t.Errorf("saved labels = %v, want [fix retries]", saved)
	}
}
