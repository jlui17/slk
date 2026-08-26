package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gammons/slk/internal/slack/boot"
	"github.com/gammons/slk/internal/slack/edge"
)

// callIndex returns the index of name's first occurrence in the call
// log, or -1 if it was never called.
func (f *fakeDeps) callIndex(name string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, c := range f.calls {
		if c == name {
			return i
		}
	}
	return -1
}

// overlapGate synchronizes the overlap tests: one dependency blocks in
// wait until another dependency's call opens the gate. A correct
// overlapped boot opens the gate within microseconds; a re-serialized
// one deadlocks, which the timeout converts into a visible failure
// instead of a hung test. 2s is failure-path-only cost — the success
// path never sleeps.
type overlapGate struct {
	release  chan struct{}
	once     sync.Once
	timedOut atomic.Bool
}

func newOverlapGate() *overlapGate {
	return &overlapGate{release: make(chan struct{})}
}

func (g *overlapGate) open() { g.once.Do(func() { close(g.release) }) }

func (g *overlapGate) wait() {
	select {
	case <-g.release:
	case <-time.After(2 * time.Second):
		g.timedOut.Store(true)
	}
}

// gatedCounts blocks the counts call until the gate opens.
type gatedCounts struct {
	inner CountsFetcher
	gate  *overlapGate
}

func (c gatedCounts) Counts(ctx context.Context) (Counts, error) {
	c.gate.wait()
	return c.inner.Counts(ctx)
}

// gatedViewer blocks the view call until the gate opens.
type gatedViewer struct {
	inner Viewer
	gate  *overlapGate
}

func (v gatedViewer) ConversationsView(ctx context.Context, channelID string) (*boot.ViewResult, error) {
	v.gate.wait()
	return v.inner.ConversationsView(ctx, channelID)
}

// openingViewer opens the gate when the view is called.
type openingViewer struct {
	inner Viewer
	gate  *overlapGate
}

func (v openingViewer) ConversationsView(ctx context.Context, channelID string) (*boot.ViewResult, error) {
	v.gate.open()
	return v.inner.ConversationsView(ctx, channelID)
}

// openingRevalidator opens the gate when channels/info is called.
type openingRevalidator struct {
	Revalidator
	gate *overlapGate
}

func (r openingRevalidator) ChannelsInfo(ctx context.Context, teamID string, updatedIDs map[string]int64) (edge.ChannelsInfoResult, error) {
	r.gate.open()
	return r.Revalidator.ChannelsInfo(ctx, teamID, updatedIDs)
}

// slowViewer delays the view long enough that a users pass launched
// concurrently with the open — instead of after it — lands in the call
// log first.
type slowViewer struct {
	inner Viewer
	delay time.Duration
}

func (v slowViewer) ConversationsView(ctx context.Context, channelID string) (*boot.ViewResult, error) {
	time.Sleep(v.delay)
	return v.inner.ConversationsView(ctx, channelID)
}

// slowHistorian is slowViewer for the fallback path.
type slowHistorian struct {
	inner Historian
	delay time.Duration
}

func (h slowHistorian) HistoryWithVersions(ctx context.Context, channelID string, cached map[string]string) (History, error) {
	time.Sleep(h.delay)
	return h.inner.HistoryWithVersions(ctx, channelID, cached)
}

func TestRun_UsersInfoWaitsForTheChannelOpen(t *testing.T) {
	// The one ordering constraint the overlap keeps: the users pass
	// revalidates the authors conversations.view returned, so racing
	// it ahead of (or alongside) the open scopes users/info to the DM
	// counterparties alone. The slow open makes a mis-ordered pass
	// land in the call log first rather than winning by luck.
	for _, tc := range []struct {
		name     string
		prepare  func(*fakeDeps)
		openEnd  string           // the call that completes the open on this path
		wantSent map[string]int64 // the fallback discards the view's users, so only the DM counterparties remain in scope there
	}{
		{"view honoured", func(f *fakeDeps) {
			f.viewChannelID = "C_WANT"
			f.deps.View = slowViewer{inner: f, delay: 50 * time.Millisecond}
		}, callView, wantUsersInfoSent()},
		{"fallback", func(f *fakeDeps) {
			f.viewChannelID = "C_LASTVIEWED"
			f.deps.History = slowHistorian{inner: f, delay: 50 * time.Millisecond}
		}, callHistory, map[string]int64{"U_ALICE": 1783337533030, "U_BOB": 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeDeps()
			f.deps.OpenChannelID = "C_WANT"
			tc.prepare(f)

			if _, err := Run(context.Background(), f.Deps()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			iOpen, iUsers := f.callIndex(tc.openEnd), f.callIndex(callUsersInfo)
			if iUsers < 0 {
				t.Fatalf("users/info was never called (sequence: %v)", f.calls)
			}
			if iUsers < iOpen {
				t.Errorf("users/info at index %d preceded %s at %d; it must wait for the open, whose authors are its id set (sequence: %v)",
					iUsers, tc.openEnd, iOpen, f.calls)
			}
			if !reflect.DeepEqual(f.usersInfoSent, tc.wantSent) {
				t.Errorf("users/info was sent %v; want %v — the id set is what the completed open left in scope", f.usersInfoSent, tc.wantSent)
			}
		})
	}
}

func TestRun_SlowCountsDoesNotDelayTheChannelOpen(t *testing.T) {
	// The point of the overlap: counts and the open are independent
	// round trips. counts here refuses to answer until the view has
	// been issued, so a boot that chains the open behind counts
	// deadlocks into the gate's timeout.
	f := openedFake()
	gate := newOverlapGate()
	f.deps.Counts = gatedCounts{inner: f, gate: gate}
	f.deps.View = openingViewer{inner: f, gate: gate}

	res, err := Run(context.Background(), f.Deps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gate.timedOut.Load() {
		t.Fatal("conversations.view was not issued while counts was in flight; the open is serialized behind counts")
	}
	if !res.CountsOK || !reflect.DeepEqual(res.Counts, cannedCounts()) {
		t.Errorf("Counts = %#v (ok=%v); want %#v — overlapping must not lose the counts result", res.Counts, res.CountsOK, cannedCounts())
	}
	if !reflect.DeepEqual(rawStrings(res.Messages), rawStrings(cannedViewResult().History.Messages)) {
		t.Errorf("Messages = %q; want the view's — overlapping must not lose the open", rawStrings(res.Messages))
	}
}

func TestRun_SlowOpenDoesNotDelayChannelsInfo(t *testing.T) {
	// channels/info's id set is userBoot's channels + ims — it does
	// not need the open. The view here refuses to answer until
	// channels/info has been issued, so a boot that chains the
	// channels pass behind the open deadlocks into the gate's timeout.
	f := openedFake()
	gate := newOverlapGate()
	f.deps.View = gatedViewer{inner: f, gate: gate}
	f.deps.Revalidate = openingRevalidator{Revalidator: f, gate: gate}

	if _, err := Run(context.Background(), f.Deps()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if gate.timedOut.Load() {
		t.Fatal("channels/info was not issued while the open was in flight; the channels pass is serialized behind the open")
	}
	if !reflect.DeepEqual(f.channelUpdates, wantChannelUpdates()) {
		t.Errorf("channel updates = %#v; want %#v — overlapping must not lose the channels pass", f.channelUpdates, wantChannelUpdates())
	}
}

func TestRun_CountsFailureDoesNotDisturbTheOverlappedPhases(t *testing.T) {
	// The degrade semantics with a channel open, now that the phases
	// share a boot instead of running one after another: a failed
	// counts still means CountsOK=false and a zero Counts, while the
	// open and both revalidation passes land in full.
	f := openedFake()
	f.countsErr = errors.New("ratelimited")

	res, err := Run(context.Background(), f.Deps())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.CountsOK {
		t.Error("CountsOK is true after a failed counts call")
	}
	if !reflect.DeepEqual(res.Counts, Counts{}) {
		t.Errorf("Result.Counts = %#v; want the zero value", res.Counts)
	}
	if !reflect.DeepEqual(rawStrings(res.Messages), rawStrings(cannedViewResult().History.Messages)) {
		t.Errorf("Messages = %q; want the view's — a counts failure is not the open's problem", rawStrings(res.Messages))
	}
	if !reflect.DeepEqual(f.usersInfoSent, wantUsersInfoSent()) {
		t.Errorf("users/info was sent %v; want %v", f.usersInfoSent, wantUsersInfoSent())
	}
	if !reflect.DeepEqual(f.channelsInfoSent, wantChannelsInfoSent()) {
		t.Errorf("channels/info was sent %v; want %v", f.channelsInfoSent, wantChannelsInfoSent())
	}
}
