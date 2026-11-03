package slackclient

import (
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// quietServer says hello on each socket and then sends nothing of its
// own, as Slack's gateway does on a quiet workspace. It answers the
// first `answered` app-level pings with Slack's pong and ignores the
// rest; a negative `answered` answers every ping.
type quietServer struct {
	*openSocketServer
	pings    atomic.Int32
	lastSent atomic.Int64 // unix nanos of the server's last frame
}

func newQuietServer(t *testing.T, answered int32) *quietServer {
	t.Helper()
	s := &quietServer{}
	s.openSocketServer = newOpenSocketServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"hello"}`))
		s.lastSent.Store(time.Now().UnixNano())
		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var ping struct {
				Type string `json:"type"`
				ID   int    `json:"id"`
			}
			if json.Unmarshal(data, &ping) != nil || ping.Type != "ping" {
				continue
			}
			n := s.pings.Add(1)
			if answered >= 0 && n > answered {
				continue
			}
			pong, _ := json.Marshal(map[string]any{"type": "pong", "reply_to": ping.ID})
			if conn.WriteMessage(websocket.TextMessage, pong) != nil {
				return
			}
			s.lastSent.Store(time.Now().UnixNano())
		}
	})
	return s
}

// shortReadTimeout shortens the read deadline for one test.
func shortReadTimeout(t *testing.T, d time.Duration) {
	t.Helper()
	old := wsReadTimeout
	wsReadTimeout = d
	t.Cleanup(func() { wsReadTimeout = old })
}

// TestQuietSocketStaysUp pins that a healthy socket on a quiet
// workspace stays up: Slack sends nothing after hello for longer than
// the read deadline, and the keepalive's answered pings keep the
// socket open across several deadlines.
func TestQuietSocketStaysUp(t *testing.T) {
	const timeout = 400 * time.Millisecond
	shortReadTimeout(t, timeout)
	srv := newQuietServer(t, -1)
	c := &Client{wsBaseURL: "ws://" + srv.Listener.Addr().String()}
	start := time.Now()
	if err := c.StartWebSocket(&mockEventHandler{}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = c.StopWebSocket(); <-c.WsDone() }()

	select {
	case <-c.WsDone():
		t.Fatalf("quiet socket ended after %v, want it up across 4 deadlines of %v (server got %d pings)",
			time.Since(start).Round(time.Millisecond), timeout, srv.pings.Load())
	case <-time.After(4 * timeout):
	}
	if got := srv.pings.Load(); got < 4 {
		t.Errorf("server got %d pings in 4 deadlines, want at least 4", got)
	}
}

// TestUnansweredPingsCloseSocket pins that the read deadline still
// finds a dead socket: once the server stops answering, the socket
// closes one deadline after the server's last frame.
func TestUnansweredPingsCloseSocket(t *testing.T) {
	const timeout = 400 * time.Millisecond
	shortReadTimeout(t, timeout)
	srv := newQuietServer(t, 3)
	c := &Client{wsBaseURL: "ws://" + srv.Listener.Addr().String()}
	if err := c.StartWebSocket(&mockEventHandler{}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	select {
	case <-c.WsDone():
	case <-time.After(10 * timeout):
		t.Fatalf("socket still up %v after connect, with pings unanswered after the 3rd", 10*timeout)
	}
	silent := time.Since(time.Unix(0, srv.lastSent.Load()))
	if silent < timeout-50*time.Millisecond || silent > timeout+250*time.Millisecond {
		t.Errorf("socket closed %v after the server's last frame, want one deadline (%v)", silent, timeout)
	}
	if got := srv.pings.Load(); got < 4 {
		t.Errorf("server got %d pings, want the 3 it answered and at least one more", got)
	}
	srv.waitOpen(t, 0, "after the dead socket's read loop ended")
}

// TestPingWaitRange pins the keepalive's spacing at the live 60s
// deadline: every wait in [15s, 25s), so two pings fit in one deadline,
// and spread across that range, so slks whose sockets opened together
// do not ping in lockstep.
func TestPingWaitRange(t *testing.T) {
	lo, hi := 25*time.Second, 15*time.Second
	for range 2000 {
		w := pingWait(60 * time.Second)
		if w < 15*time.Second || w >= 25*time.Second {
			t.Fatalf("pingWait(60s) = %v, want within [15s, 25s)", w)
		}
		lo, hi = min(lo, w), max(hi, w)
	}
	if lo > 16*time.Second || hi < 24*time.Second {
		t.Errorf("2000 waits span [%v, %v], want them spread across [15s, 25s)", lo, hi)
	}
}

// TestNoPingAfterClose pins that the keepalive stops with its socket:
// once WsDone closes, no further ping is attempted.
func TestNoPingAfterClose(t *testing.T) {
	const timeout = 400 * time.Millisecond
	shortReadTimeout(t, timeout)
	srv := newQuietServer(t, -1)
	c := &Client{wsBaseURL: "ws://" + srv.Listener.Addr().String()}
	if err := c.StartWebSocket(&mockEventHandler{}); err != nil {
		t.Fatalf("connect: %v", err)
	}
	o := c.wsOutcome
	deadline := time.Now().Add(2 * timeout)
	for o.pings.Load() < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("keepalive sent %d pings in %v, want 2 before closing", o.pings.Load(), 2*timeout)
		}
		time.Sleep(5 * time.Millisecond)
	}
	_ = c.StopWebSocket()
	<-c.WsDone()
	closed := o.pings.Load()
	time.Sleep(3 * timeout) // several ping intervals
	if got := o.pings.Load(); got != closed {
		t.Errorf("keepalive sent %d pings after the socket closed", got-closed)
	}
	if o.pongs.Load() == 0 {
		t.Errorf("event owner counted no pongs; want the server's answers counted")
	}
}
