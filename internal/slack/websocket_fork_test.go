package slackclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// openSocketServer is a WebSocket server that counts the sockets it
// still holds open. A socket counts as open from the upgrade until the
// client closes its end (the server reads EOF): the server never closes
// one itself, the way Slack can keep a socket a client stopped reading.
// onOpen runs on each new socket.
type openSocketServer struct {
	*httptest.Server
	open atomic.Int32
}

func newOpenSocketServer(t *testing.T, onOpen func(*websocket.Conn)) *openSocketServer {
	t.Helper()
	s := &openSocketServer{}
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		s.open.Add(1)
		defer s.open.Add(-1)
		defer conn.Close()
		onOpen(conn)
		// Drain raw bytes (the client's close frame included) until the
		// client closes the TCP connection.
		_, _ = io.Copy(io.Discard, conn.UnderlyingConn())
	}))
	t.Cleanup(s.Close)
	return s
}

func (s *openSocketServer) waitOpen(t *testing.T, want int32, why string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for s.open.Load() != want {
		if time.Now().After(deadline) {
			t.Fatalf("%s: server holds %d open sockets, want %d", why, s.open.Load(), want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestReadLoopExitClosesSocket pins that a socket whose read loop
// ended is closed, not left for the garbage collector: Slack still
// counts a socket slk stopped reading, and on a reconnect the next
// socket joins it. The server breaks each socket's read loop with a
// frame the client rejects (an unknown opcode) and keeps its own end
// open, so a client that does not close its end leaks one socket per
// reconnect.
func TestReadLoopExitClosesSocket(t *testing.T) {
	srv := newOpenSocketServer(t, func(conn *websocket.Conn) {
		// FIN + opcode 3 (reserved), zero length: a protocol error
		// for the client's reader.
		_, _ = conn.UnderlyingConn().Write([]byte{0x83, 0x00})
	})
	c := &Client{wsBaseURL: "ws://" + srv.Listener.Addr().String()}

	for i := 1; i <= 3; i++ {
		if err := c.StartWebSocket(&mockEventHandler{}); err != nil {
			t.Fatalf("connect %d: %v", i, err)
		}
		select {
		case <-c.WsDone():
		case <-time.After(5 * time.Second):
			t.Fatalf("connect %d: read loop did not end on the rejected frame", i)
		}
		srv.waitOpen(t, 0, fmt.Sprintf("after the read loop of connect %d ended", i))
	}
}

// runAgainst runs a ConnectionManager against srv until it has
// reported n reconnect waits, and returns how long each wait was.
func runAgainst(t *testing.T, srv *openSocketServer, n int) []time.Duration {
	t.Helper()
	c := &Client{wsBaseURL: "ws://" + srv.Listener.Addr().String()}
	cm := NewConnectionManager(c, &mockEventHandler{})
	waits := make(chan time.Duration, n)
	cm.OnReconnectWait = func(retryAt time.Time, _ int) {
		select {
		case waits <- time.Until(retryAt):
		default:
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); cm.Run(ctx) }()
	defer func() { cancel(); <-done }()

	var got []time.Duration
	for len(got) < n {
		select {
		case w := <-waits:
			got = append(got, w)
		case <-time.After(20 * time.Second):
			t.Fatalf("got %d reconnect waits, want %d: %v", len(got), n, got)
		}
	}
	return got
}

// ratelimitedServer accepts each socket, sends frame (Slack's
// Ratelimited error) before any hello, and closes the socket, as Slack
// does when the user holds too many sockets.
func ratelimitedServer(t *testing.T, frame string) *openSocketServer {
	return newOpenSocketServer(t, func(conn *websocket.Conn) {
		_ = conn.WriteMessage(websocket.TextMessage, []byte(frame))
		_ = conn.UnderlyingConn().Close()
	})
}

// TestRatelimitedSocketsBackOff pins that a socket Slack accepts and
// then closes without a hello does not count as a connection: the
// reconnect waits keep doubling toward the cap instead of starting over
// at 1s after every upgrade.
func TestRatelimitedSocketsBackOff(t *testing.T) {
	t.Parallel()
	srv := ratelimitedServer(t, `{"type":"error","error":{"msg":"Ratelimited","code":17,"source":"gatewayserver-test"}}`)
	waits := runAgainst(t, srv, 3)
	for i, max := range []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second} {
		// Waits are jittered down to half the backoff; 50ms slack for
		// the time between scheduling a wait and reading it.
		if min := max / 2; waits[i] < min-50*time.Millisecond || waits[i] > max {
			t.Errorf("wait %d = %v, want within [%v, %v] (all waits %v)", i+1, waits[i], min, max, waits)
		}
	}
}

// TestRatelimitedRetryAfterIsHonored pins that a retry_after on an
// error frame sets the shortest wait before the next dial.
func TestRatelimitedRetryAfterIsHonored(t *testing.T) {
	t.Parallel()
	srv := ratelimitedServer(t, `{"type":"error","error":{"msg":"Ratelimited","code":17,"retry_after":3}}`)
	waits := runAgainst(t, srv, 1)
	if waits[0] < 3*time.Second-50*time.Millisecond {
		t.Errorf("wait after retry_after 3 = %v, want at least 3s", waits[0])
	}
}

// TestReconnectWait pins the jitter range (the upper half of the
// backoff, so the cap holds) and the retry_after floor.
func TestReconnectWait(t *testing.T) {
	seen := map[time.Duration]bool{}
	for range 200 {
		w := reconnectWait(30*time.Second, 0)
		if w < 15*time.Second || w > 30*time.Second {
			t.Fatalf("reconnectWait(30s, 0) = %v, want within [15s, 30s]", w)
		}
		seen[w] = true
	}
	if len(seen) < 2 {
		t.Fatal("reconnectWait returned one value 200 times: no jitter")
	}
	if w := reconnectWait(1*time.Second, 10*time.Second); w != 10*time.Second {
		t.Fatalf("reconnectWait(1s, retry_after 10s) = %v, want 10s", w)
	}
}

// TestSocketProvedHealthy pins what resets the backoff: a hello, then
// healthySocketAge of uptime.
func TestSocketProvedHealthy(t *testing.T) {
	end := time.Now()
	cases := []struct {
		name    string
		helloAt time.Time
		want    bool
	}{
		{"no hello (Ratelimited)", time.Time{}, false},
		{"hello, then closed 10s later", end.Add(-10 * time.Second), false},
		{"hello, then up for healthySocketAge", end.Add(-healthySocketAge), true},
	}
	for _, tc := range cases {
		o := &wsOutcome{helloAt: tc.helloAt}
		if got := o.provedHealthy(end); got != tc.want {
			t.Errorf("%s: provedHealthy = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestObserveFrames pins which frames the outcome records.
func TestObserveFrames(t *testing.T) {
	o := &wsOutcome{}
	o.observe([]byte(`{"type":"message","text":"hello"}`))
	if !o.helloAt.IsZero() {
		t.Fatal("a message saying hello was taken for Slack's hello")
	}
	o.observe([]byte(`{"type":"hello"}`))
	if o.helloAt.IsZero() {
		t.Fatal("hello frame not recorded")
	}
	o.observe([]byte(`{"type":"error","error":{"msg":"Ratelimited","code":17}}`))
	if o.errorMsg != "Ratelimited" || o.retryAfter != 0 {
		t.Fatalf("Ratelimited without retry_after: errorMsg %q, retryAfter %v; want Ratelimited, 0", o.errorMsg, o.retryAfter)
	}
	o.observe([]byte(`{"type":"error","retry_after":7,"error":{"msg":"Ratelimited","code":17}}`))
	if o.retryAfter != 7*time.Second {
		t.Fatalf("top-level retry_after 7: retryAfter = %v, want 7s", o.retryAfter)
	}
}
