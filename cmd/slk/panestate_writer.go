package main

import (
	"log"
	"os"
	"sync"

	"github.com/gammons/slk/internal/cache"
)

// paneStateKey identifies this pane's persisted state across restarts.
// The key is the launch env's pane id: constant for the shell's
// lifetime, and herdr re-exports an un-moved pane's public id across
// server restarts, so a restarted pane reads the state its previous
// incarnation wrote. A cross-workspace move re-keys the pane's future
// incarnations (the old row is orphaned and state starts fresh);
// outside herdr every instance shares one slot.
func paneStateKey() string {
	if id := os.Getenv("HERDR_PANE_ID"); id != "" {
		return id
	}
	return "default"
}

// paneStateWriter serializes pane-state writes on one goroutine,
// coalescing bursts to the newest snapshot. Ordering matters here in a
// way it doesn't for channel visits: a per-write goroutine could land
// (channel, thread) before the (channel, "") that preceded it and
// persist a closed thread as open.
type paneStateWriter struct {
	db  *cache.DB
	key string

	mu     sync.Mutex
	latest *cache.PaneState

	wake chan struct{}
	done chan struct{}
}

func newPaneStateWriter(db *cache.DB, key string) *paneStateWriter {
	w := &paneStateWriter{
		db:   db,
		key:  key,
		wake: make(chan struct{}, 1),
		done: make(chan struct{}),
	}
	go func() {
		defer close(w.done)
		for range w.wake {
			w.flush()
		}
		w.flush()
	}()
	return w
}

// Record replaces the pending snapshot and nudges the writer. Cheap
// enough to call from the UI update loop.
func (w *paneStateWriter) Record(s cache.PaneState) {
	w.mu.Lock()
	w.latest = &s
	w.mu.Unlock()
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// Close flushes the last pending snapshot and stops the writer. Call
// after the UI loop has exited; Record must not be called afterwards.
func (w *paneStateWriter) Close() {
	close(w.wake)
	<-w.done
}

func (w *paneStateWriter) flush() {
	w.mu.Lock()
	s := w.latest
	w.latest = nil
	w.mu.Unlock()
	if s == nil {
		return
	}
	if err := w.db.RecordPaneState(w.key, *s); err != nil {
		log.Printf("warning: recording pane state: %v", err)
	}
}
