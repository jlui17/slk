package slackclient

import (
	"net/http"

	"github.com/gammons/slk/internal/slackhttp"
	"github.com/gorilla/websocket"
)

// TestOption is a test-only override applied by NewTestClient.
type TestOption func(*Client)

// NewTestClient is NewClient plus test-only overrides, for tests in
// other packages that need a real Client pointed at a fake backend
// (see internal/slacktest).
func NewTestClient(xoxcToken, dCookie string, opts ...TestOption) *Client {
	c := NewClient(xoxcToken, dCookie)
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// WithInnerTransport replaces the BrowserTransport's inner transport,
// leaving the browser-header and envelope decoration in place. This is
// the primary harness seam: production URLs are preserved and the
// redirect to a test server happens at the dial, because
// deriveAPIBaseURL and slackhttp's host gates only trust *.slack.com
// hosts.
func WithInnerTransport(inner http.RoundTripper) TestOption {
	return func(c *Client) {
		c.httpClient.Transport.(*slackhttp.BrowserTransport).Inner = inner
	}
}

// WithWSDialer replaces the dialer StartWebSocket builds, so the
// production wss URL can be redirected to a test server at the dial —
// the WebSocket analogue of WithInnerTransport.
func WithWSDialer(d *websocket.Dialer) TestOption {
	return func(c *Client) { c.wsDialer = d }
}

// wsDialerOrDefault keeps the auth cookie jar on injected dialers too:
// the d cookie is part of the production wire shape the harness exists
// to preserve, so a jar-less test dialer gets the real jar merged in.
func (c *Client) wsDialerOrDefault() *websocket.Dialer {
	if c.wsDialer != nil {
		if c.wsDialer.Jar == nil {
			c.wsDialer.Jar = newCookieJar(c.cookie)
		}
		return c.wsDialer
	}
	return &websocket.Dialer{Jar: newCookieJar(c.cookie)}
}
