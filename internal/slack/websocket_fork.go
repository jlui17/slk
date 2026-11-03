package slackclient

import (
	"bytes"
	"encoding/json"
	"sync/atomic"
	"time"
)

// healthySocketAge is how long a socket must stay up after Slack's
// hello before its end resets the reconnect backoff. It matches
// StartWebSocket's 60s read deadline: a socket that outlived one full
// deadline window has carried live traffic, while Slack's rejections
// (Ratelimited, then a close about 5s later) end well inside it.
const healthySocketAge = 60 * time.Second

// wsOutcome is what one socket proved before it ended: when Slack said
// hello on it, and the error frame it got, with the retry_after that
// frame asked for. The event owner writes it; Run reads it after
// WsDone, which orders the two. The read loop's error, and the
// keepalive's ping and pong counts, are there for the debug log.
type wsOutcome struct {
	helloAt    time.Time
	errorMsg   string
	retryAfter time.Duration

	openedAt time.Time
	readErr  error
	pings    atomic.Int32
	pongs    atomic.Int32
}

// observe records the hello and error frames Run's backoff depends on.
func (o *wsOutcome) observe(frame []byte) {
	if !bytes.Contains(frame, []byte(`"hello"`)) && !bytes.Contains(frame, []byte(`"error"`)) {
		return
	}
	var evt struct {
		Type  string `json:"type"`
		Error struct {
			Msg        string  `json:"msg"`
			RetryAfter float64 `json:"retry_after"`
		} `json:"error"`
		RetryAfter float64 `json:"retry_after"`
	}
	if json.Unmarshal(frame, &evt) != nil {
		return
	}
	switch evt.Type {
	case "hello":
		o.helloAt = time.Now()
	case "error":
		// Slack's Ratelimited frame seen live carries no retry_after;
		// either placement is honored should one arrive.
		o.errorMsg = evt.Error.Msg
		secs := max(evt.RetryAfter, evt.Error.RetryAfter)
		o.retryAfter = time.Duration(secs * float64(time.Second))
	}
}

// provedHealthy reports whether the socket got Slack's hello and then
// stayed up for healthySocketAge.
func (o *wsOutcome) provedHealthy(ended time.Time) bool {
	return !o.helloAt.IsZero() && ended.Sub(o.helloAt) >= healthySocketAge
}
