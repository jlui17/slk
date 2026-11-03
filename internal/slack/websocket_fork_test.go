package slackclient

import (
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
