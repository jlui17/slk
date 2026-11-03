package slackclient

import (
	"bytes"
	"encoding/json"
	"math/rand/v2"
	"time"

	"github.com/gammons/slk/internal/debuglog"
	"github.com/gorilla/websocket"
)

// wsReadTimeout is how long a socket may go without a frame or a pong
// before StartWebSocket's read loop gives it up as dead. A variable so
// tests can shorten it.
var wsReadTimeout = 60 * time.Second

// pingWait is how long the keepalive waits before its next ping: a
// random point in [timeout/4, timeout/4+timeout/6), 15-25s at the 60s
// deadline. Two pings fit in one deadline, so one lost pong does not
// end a healthy socket, and slks whose sockets opened in the same
// instant drift apart instead of pinging Slack together.
func pingWait(timeout time.Duration) time.Duration {
	return timeout/4 + rand.N(timeout/6)
}

// keepAlive sends Slack's application-level ping, {"type":"ping","id":N},
// on conn every pingWait until the returned stop is called. Slack
// answers each with {"type":"pong","reply_to":N}, and that frame, like
// any other, extends the read deadline. stop waits for the pinger to
// exit, so nothing is written after it returns.
func (c *Client) keepAlive(conn *websocket.Conn, timeout time.Duration) (stop func()) {
	o := c.wsOutcome
	debuglog.WS("team=%s socket open", c.teamID)
	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		for id := 1; ; id++ {
			t := time.NewTimer(pingWait(timeout))
			select {
			case <-done:
				t.Stop()
				return
			case <-t.C:
			}
			o.pings.Add(1)
			c.wsMu.Lock()
			err := conn.WriteJSON(map[string]any{"type": "ping", "id": id})
			c.wsMu.Unlock()
			debuglog.WS("team=%s ping id=%d err=%v pongs=%d", c.teamID, id, err, o.pongs.Load())
			if err != nil {
				return
			}
		}
	}()
	return func() {
		close(done)
		<-exited
	}
}

// pong reports whether frame is Slack's answer to a keepalive ping,
// and counts it. The read loop extended the deadline when the frame
// arrived, so the event handler has nothing to do with it.
func (o *wsOutcome) pong(frame []byte) bool {
	if !bytes.Contains(frame, []byte(`"pong"`)) {
		return false
	}
	var evt struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(frame, &evt) != nil || evt.Type != "pong" {
		return false
	}
	o.pongs.Add(1)
	return true
}
