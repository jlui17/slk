package main

import (
	"log"

	"github.com/gammons/slk/internal/cache"
)

// agentStateReportWriter appends agent-state reports on one goroutine, in
// the order they were recorded: the log is read by row order, which a
// per-write goroutine could scramble.
type agentStateReportWriter struct {
	rows chan cache.AgentStateReport
	done chan struct{}
}

func newAgentStateReportWriter(db *cache.DB) *agentStateReportWriter {
	w := &agentStateReportWriter{
		rows: make(chan cache.AgentStateReport, 64),
		done: make(chan struct{}),
	}
	go func() {
		defer close(w.done)
		for r := range w.rows {
			if err := db.InsertAgentStateReport(r); err != nil {
				log.Printf("warning: %v", err)
			}
		}
	}()
	return w
}

// Record queues r without ever blocking the UI update loop: when the
// queue is full (storage stalled), the row is dropped.
func (w *agentStateReportWriter) Record(r cache.AgentStateReport) {
	select {
	case w.rows <- r:
	default:
		log.Printf("warning: agent state report dropped, writer queue is full")
	}
}

// Close writes what is still queued and stops the writer. Call after the
// UI loop has exited; Record must not be called afterwards.
func (w *agentStateReportWriter) Close() {
	close(w.rows)
	<-w.done
}
