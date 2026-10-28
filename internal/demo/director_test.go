package demo

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type mark string

func emit(s string) Act { return func(*Scene) []tea.Msg { return []tea.Msg{mark(s)} } }

type recorder struct {
	mu   sync.Mutex
	msgs []tea.Msg
}

func (r *recorder) send(m tea.Msg) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, m)
}

func (r *recorder) all() []tea.Msg {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.msgs)
}

// waitFor polls until at least n messages have arrived.
func (r *recorder) waitFor(t *testing.T, n int) []tea.Msg {
	t.Helper()
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(5 * time.Millisecond) {
		if got := r.all(); len(got) >= n {
			return got
		}
	}
	t.Fatalf("got %d messages, want at least %d: %#v", len(r.all()), n, r.all())
	return nil
}

// settle waits until nothing new has arrived for 50ms (3s at most).
func (r *recorder) settle() []tea.Msg {
	last := -1
	for i := 0; i < 60; i++ {
		time.Sleep(50 * time.Millisecond)
		if n := len(r.all()); n == last {
			break
		} else {
			last = n
		}
	}
	return r.all()
}

// delays is an instant clock that records every delay it was asked for.
type delays struct {
	mu  sync.Mutex
	got []time.Duration
}

func (dl *delays) instant(d time.Duration) <-chan time.Time {
	dl.mu.Lock()
	dl.got = append(dl.got, d)
	dl.mu.Unlock()
	ch := make(chan time.Time, 1)
	ch <- time.Time{}
	return ch
}

func (dl *delays) all() []time.Duration {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return slices.Clone(dl.got)
}

// start runs d until the test ends. The returned stop cancels it and
// fails the test if run does not return promptly.
func start(t *testing.T, d *Director, send func(tea.Msg)) (stop func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.run(ctx, send); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("director did not stop after cancel")
		}
	}
}

func TestDirectorPlaysStepsInOrderAfterTheirDelays(t *testing.T) {
	d := newDirector(fixtureWorld(), []Rule{{When: on(EventChannelOpened, "C1"), Steps: []Step{
		{After: time.Second, Act: emit("a")},
		{After: 2 * time.Second, Act: emit("b")},
		{After: 3 * time.Second, Act: emit("c")},
	}}})
	dl := &delays{}
	d.after = dl.instant
	rec := &recorder{}
	start(t, d, rec.send)

	d.observe(Event{Kind: EventChannelOpened, ChannelID: "C1"})

	if got, want := rec.waitFor(t, 3), []tea.Msg{mark("a"), mark("b"), mark("c")}; !slices.Equal(got, want) {
		t.Errorf("messages = %v, want %v", got, want)
	}
	if got, want := dl.all(), []time.Duration{time.Second, 2 * time.Second, 3 * time.Second}; !slices.Equal(got, want) {
		t.Errorf("delays = %v, want %v", got, want)
	}
}

func TestDirectorOnceRulesPlayOnce(t *testing.T) {
	d := newDirector(fixtureWorld(), []Rule{
		{When: on(EventChannelOpened, "C1"), Once: true, Steps: []Step{{Act: emit("once")}}},
		{When: on(EventChannelOpened, "C2"), Steps: []Step{{Act: emit("done")}}},
	})
	d.after = (&delays{}).instant
	rec := &recorder{}
	start(t, d, rec.send)

	d.observe(Event{Kind: EventChannelOpened, ChannelID: "C1"})
	d.observe(Event{Kind: EventChannelOpened, ChannelID: "C1"})
	d.observe(Event{Kind: EventChannelOpened, ChannelID: "C2"})

	got := rec.settle()
	if n := len(slices.DeleteFunc(slices.Clone(got), func(m tea.Msg) bool { return m != mark("once") })); n != 1 {
		t.Errorf("once-rule played %d times: %v", n, got)
	}
}

func TestDirectorFiresStartOnItsOwn(t *testing.T) {
	d := newDirector(fixtureWorld(), []Rule{{When: on(EventStart, anyChannel), Steps: []Step{{Act: emit("hello")}}}})
	d.after = (&delays{}).instant
	rec := &recorder{}
	start(t, d, rec.send)
	if got := rec.waitFor(t, 1); got[0] != mark("hello") {
		t.Errorf("got %v", got)
	}
}

func TestDirectorCancelStopsPendingSteps(t *testing.T) {
	d := newDirector(fixtureWorld(), []Rule{{When: on(EventStart, anyChannel), Steps: []Step{{After: time.Hour, Act: emit("late")}}}})
	d.after = func(time.Duration) <-chan time.Time { return make(chan time.Time) } // never fires
	rec := &recorder{}
	stop := start(t, d, rec.send)
	time.Sleep(20 * time.Millisecond)
	stop()
	if got := rec.all(); len(got) != 0 {
		t.Errorf("a step fired after cancel: %v", got)
	}
}

func TestObserveNeverBlocks(t *testing.T) {
	d := newDirector(fixtureWorld(), nil)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			d.observe(Event{Kind: EventMessageSent})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("observe blocked with nobody running the director")
	}
}
