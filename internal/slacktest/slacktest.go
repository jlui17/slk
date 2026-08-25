// Package slacktest is a credential-free fake Slack backend for tests:
// an HTTP server answering the workspace Web API and edgeapi paths from
// a synthetic corpus (overridable per test, every request recorded),
// and a WebSocket server that completes the upgrade, lets a test inject
// event frames, and exposes the frames the client sends.
//
// The redirect contract: a client keeps building production
// *.slack.com / edgeapi.slack.com / wss-primary.slack.com URLs — so
// host-gated behavior (deriveAPIBaseURL, browser-header decoration)
// runs exactly as in production — and Transport / WSDialer land the
// bytes here at the dial. Wire one up with slackclient.NewTestClient
// (internal/slack/testing_fork.go).
package slacktest

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// Request is one request the fake backend captured. Form is populated
// for form-encoded Web API calls, Body always holds the raw bytes
// (edgeapi requests are JSON).
type Request struct {
	Path   string
	Query  url.Values
	Form   url.Values
	Header http.Header
	Body   []byte
}

type Server struct {
	t   testing.TB
	api *httptest.Server
	ws  *httptest.Server

	mu        sync.Mutex
	responses map[string]string
	reqs      []Request
	conns     []*websocket.Conn

	connCh       chan *websocket.Conn
	eventConn    *websocket.Conn
	clientFrames chan []byte
}

const wsWait = 5 * time.Second

func New(t testing.TB) *Server {
	s := &Server{
		t:            t,
		responses:    defaultResponses(),
		connCh:       make(chan *websocket.Conn, 4),
		clientFrames: make(chan []byte, 64),
	}
	s.api = httptest.NewTLSServer(http.HandlerFunc(s.serveAPI))
	s.ws = httptest.NewTLSServer(http.HandlerFunc(s.serveWS))
	t.Cleanup(func() {
		s.mu.Lock()
		conns := s.conns
		s.conns = nil
		s.mu.Unlock()
		for _, c := range conns {
			_ = c.Close()
		}
		s.ws.Close()
		s.api.Close()
	})
	return s
}

// Handle registers (or overrides) the 200 JSON body served for path.
func (s *Server) Handle(path, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.responses[path] = body
}

// Requests returns every captured request, in arrival order.
func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Request, len(s.reqs))
	copy(out, s.reqs)
	return out
}

func (s *Server) RequestsTo(path string) []Request {
	var out []Request
	for _, r := range s.Requests() {
		if r.Path == path {
			out = append(out, r)
		}
	}
	return out
}

// Transport dials this server's API listener whatever host it is asked
// for. Install it with slackclient.WithInnerTransport.
func (s *Server) Transport() *http.Transport {
	return dialRedirect(s.api.Listener.Addr().String())
}

// WSDialer dials this server's WebSocket listener whatever host it is
// asked for — Transport's counterpart for the event socket. Install it
// with slackclient.WithWSDialer.
func (s *Server) WSDialer() *websocket.Dialer {
	addr := s.ws.Listener.Addr().String()
	return &websocket.Dialer{
		NetDialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		//nolint:gosec // dialing a local httptest server with a self-signed cert
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

func dialRedirect(addr string) *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		//nolint:gosec // dialing a local httptest server with a self-signed cert
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
}

// InjectEvent writes one event frame to the connected client, waiting
// up to 5s for a client to connect first.
func (s *Server) InjectEvent(frame string) {
	s.t.Helper()
	conn := s.waitConn()
	if err := conn.WriteMessage(websocket.TextMessage, []byte(frame)); err != nil {
		s.t.Fatalf("slacktest: injecting event: %v", err)
	}
}

// NextClientFrame returns the next frame the client sent over the
// WebSocket, waiting up to 5s for one to arrive.
func (s *Server) NextClientFrame() []byte {
	s.t.Helper()
	select {
	case f := <-s.clientFrames:
		return f
	case <-time.After(wsWait):
		s.t.Fatalf("slacktest: no client frame arrived")
		return nil
	}
}

// waitConn returns the newest client connection: a reconnect queues a
// fresh conn on connCh, which must supersede the cached one or injected
// frames would go to the dead socket.
func (s *Server) waitConn() *websocket.Conn {
	for {
		select {
		case conn := <-s.connCh:
			s.eventConn = conn
		default:
			if s.eventConn != nil {
				return s.eventConn
			}
			select {
			case conn := <-s.connCh:
				s.eventConn = conn
				return conn
			case <-time.After(wsWait):
				s.t.Fatalf("slacktest: no WebSocket client connected")
				return nil
			}
		}
	}
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	r.Body = io.NopCloser(bytes.NewReader(body))
	_ = r.ParseForm()

	s.mu.Lock()
	s.reqs = append(s.reqs, Request{
		Path:   r.URL.Path,
		Query:  r.URL.Query(),
		Form:   r.PostForm,
		Header: r.Header.Clone(),
		Body:   body,
	})
	resp, ok := s.responses[r.URL.Path]
	s.mu.Unlock()

	if !ok {
		s.t.Errorf("slacktest: unexpected request to %s", r.URL.Path)
		http.Error(w, `{"ok":false,"error":"unexpected_path"}`, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(resp))
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.t.Errorf("slacktest: websocket upgrade failed: %v", err)
		return
	}
	s.mu.Lock()
	s.conns = append(s.conns, conn)
	s.mu.Unlock()
	select {
	case s.connCh <- conn:
	default:
	}
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		select {
		case s.clientFrames <- msg:
		default:
		}
	}
}
