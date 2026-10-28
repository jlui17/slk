package demo

import (
	"context"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// EventKind names an action the fake services report to the director.
type EventKind int

const (
	EventStart             EventKind = iota // the director started; fired once, by the director itself
	EventChannelOpened                      // a channel's messages were fetched
	EventMarkedRead                         // the App marked a channel read
	EventThreadOpened                       // a thread's replies were fetched
	EventMessageSent                        // the user sent a message or a thread reply
	EventReactionAdded                      // the user added a reaction
	EventWorkspaceSwitched                  // the user switched workspace
)

// Event is one reported action. Fields its kind does not use are empty.
type Event struct {
	Kind      EventKind
	TeamID    string
	ChannelID string
	ThreadTS  string
}

// Act produces the messages one step sends. Acts update the World before
// returning, so later fetches agree with what the UI was told.
type Act func(*Scene) []tea.Msg

// Step waits After, then runs Act.
type Step struct {
	After time.Duration
	Act   Act
}

// Rule plays its Steps, in order, each time an event matches When. A Once
// rule plays for the first match only.
type Rule struct {
	When  func(Event) bool
	Once  bool
	Steps []Step
}

// Scene is what one playing rule carries from step to step.
type Scene struct {
	world       *World
	event       Event
	lastChannel string
	lastTS      string
}

// Director plays scenario rules against the World in response to the
// actions the fake services report.
type Director struct {
	world  *World
	rules  []Rule
	events chan Event
	after  func(time.Duration) <-chan time.Time
	fired  map[int]bool // run's goroutine only
}

func newDirector(w *World, rules []Rule) *Director {
	return &Director{
		world:  w,
		rules:  rules,
		events: make(chan Event, 64),
		after:  time.After,
		fired:  map[int]bool{},
	}
}

// observe reports ev without ever blocking. The fake services call it
// from Bubble Tea's command goroutines, sometimes before run has started;
// a full queue drops a scripted flourish rather than stall the UI.
func (d *Director) observe(ev Event) {
	select {
	case d.events <- ev:
	default:
	}
}

// run dispatches EventStart, then every observed event, until ctx is
// cancelled. It returns only after every rule it started has stopped.
func (d *Director) run(ctx context.Context, send func(tea.Msg)) {
	var wg sync.WaitGroup
	defer wg.Wait()
	d.dispatch(ctx, &wg, send, Event{Kind: EventStart})
	for {
		select {
		case <-ctx.Done():
			return
		case ev := <-d.events:
			d.dispatch(ctx, &wg, send, ev)
		}
	}
}

func (d *Director) dispatch(ctx context.Context, wg *sync.WaitGroup, send func(tea.Msg), ev Event) {
	for i, r := range d.rules {
		if !r.When(ev) || (r.Once && d.fired[i]) {
			continue
		}
		d.fired[i] = true
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.play(ctx, r, ev, send)
		}()
	}
}

func (d *Director) play(ctx context.Context, r Rule, ev Event, send func(tea.Msg)) {
	s := &Scene{world: d.world, event: ev}
	for _, st := range r.Steps {
		select {
		case <-ctx.Done():
			return
		case <-d.after(st.After):
		}
		if ctx.Err() != nil {
			return
		}
		for _, m := range st.Act(s) {
			send(m)
		}
	}
}
