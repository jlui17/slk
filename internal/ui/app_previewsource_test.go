// internal/ui/app_previewsource_test.go
//
// Coverage for previewFetchCmd's source selection: which URL it asks
// for, under which cache key, and what it does when the first choice
// fails. The command is exercised end to end against an httptest server
// through a real Fetcher and disk cache, so the assertions are on HTTP
// paths actually requested rather than on a mock's recorded calls.
package ui

import (
	stdimage "image"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/imgrender"
	"github.com/gammons/slk/internal/ui/messages"
)

// previewServer serves a decodable thumbnail at /thumb and, at
// /original, whatever originalBody says. It counts requests per path.
type previewServer struct {
	*httptest.Server
	mu   sync.Mutex
	hits map[string]int
}

func newPreviewServer(t *testing.T, originalBody []byte, originalCT string) *previewServer {
	t.Helper()
	ps := &previewServer{hits: map[string]int{}}
	thumb := makeTestPNGBytes(720, 720)
	ps.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ps.mu.Lock()
		ps.hits[r.URL.Path]++
		ps.mu.Unlock()
		switch r.URL.Path {
		case "/original":
			w.Header().Set("Content-Type", originalCT)
			w.Write(originalBody)
		default:
			w.Header().Set("Content-Type", "image/png")
			w.Write(thumb)
		}
	}))
	t.Cleanup(ps.Close)
	return ps
}

func (p *previewServer) hitCount(path string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.hits[path]
}

// previewTestApp builds an App holding one image-bearing message whose
// thumbnail and original both live on srv, wired to a real Fetcher over
// a temp-dir cache. The 120x60 terminal at 8x16 px/cell gives a 960x960
// budget, which the 720px thumb cannot cover — so the original is in
// play whenever its own dimensions allow.
func previewTestApp(t *testing.T, srv *previewServer, originalW, originalH int) (*App, string, string) {
	t.Helper()
	imgpkg.ResetUndecodableOriginalsForTest()
	t.Cleanup(imgpkg.ResetUndecodableOriginalsForTest)

	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatalf("NewCache: %v", err)
	}
	fetcher := imgpkg.NewFetcher(cache, http.DefaultClient)

	channelID, ts, _, msg := imageBearingMessage(t)
	msg.Attachments[0].Thumbs = []messages.ThumbSpec{{URL: srv.URL + "/thumb", W: 720, H: 720}}
	msg.Attachments[0].DownloadURL = srv.URL + "/original"
	msg.Attachments[0].OriginalW = originalW
	msg.Attachments[0].OriginalH = originalH

	app := NewApp()
	app.width = 120
	app.height = 60
	app.activeChannelID = channelID
	app.focusedPanel = PanelMessages
	app.messagepane.SetMessages([]messages.MessageItem{msg})
	app.SetImageFetcher(fetcher)
	app.SetImageContext(imgrender.ImageContext{
		Protocol:   imgpkg.ProtoHalfBlock,
		Fetcher:    fetcher,
		CellPixels: stdimage.Pt(8, 16),
		MaxRows:    20,
	})
	return app, channelID, ts
}

// runPreviewFetch executes previewFetchCmd and returns the resulting
// message, failing the test if the command was never built.
func runPreviewFetch(t *testing.T, app *App, channel, ts string) any {
	t.Helper()
	cmd := app.previewFetchCmd(channel, ts, 0, false)
	if cmd == nil {
		t.Fatal("previewFetchCmd returned nil")
	}
	return cmd()
}

// An undecodable original (Slack serves HEIC originals it thumbnails to
// JPEG) must fall back to the thumbnail, and must not be re-downloaded
// on the next open — the decode failure evicts the cache entry, so only
// the in-process note prevents a full re-fetch every time.
func TestPreviewFetch_UndecodableOriginalNotRefetched(t *testing.T) {
	srv := newPreviewServer(t, []byte("ftypheic not a format Go can read"), "image/heic")
	app, channelID, ts := previewTestApp(t, srv, 4000, 3000)

	if out := runPreviewFetch(t, app, channelID, ts); !isPreviewLoaded(out) {
		t.Fatalf("first open: got %T, want previewLoadedMsg via the thumbnail fallback", out)
	}
	if got := srv.hitCount("/original"); got != 1 {
		t.Fatalf("first open requested /original %d times, want 1", got)
	}

	if out := runPreviewFetch(t, app, channelID, ts); !isPreviewLoaded(out) {
		t.Fatalf("second open: got %T, want previewLoadedMsg", out)
	}
	if got := srv.hitCount("/original"); got != 1 {
		t.Errorf("second open requested /original again (total %d), want it skipped after the recorded decode failure", got)
	}
}

func isPreviewLoaded(msg any) bool {
	_, ok := msg.(previewLoadedMsg)
	return ok
}

// previewImageBounds pulls the decoded image out of a previewLoadedMsg.
func previewImageBounds(t *testing.T, msg any) stdimage.Rectangle {
	t.Helper()
	loaded, ok := msg.(previewLoadedMsg)
	if !ok {
		t.Fatalf("got %T, want previewLoadedMsg", msg)
	}
	if loaded.Img == nil {
		t.Fatal("previewLoadedMsg carried no image")
	}
	return loaded.Img.Bounds()
}

// The two sources share a file ID, so they must not share a cache key.
// Under one key the second open would be served the original's bytes
// from disk and never ask for the thumbnail at all.
func TestPreviewFetch_OriginalAndThumbUseDistinctCacheKeys(t *testing.T) {
	srv := newPreviewServer(t, makeTestPNGBytes(1600, 1200), "image/png")
	app, channelID, ts := previewTestApp(t, srv, 1600, 1200)

	// 120x60 cells at 8x16 px is a 960x960 budget: past the 720 thumb,
	// so the original wins.
	out := runPreviewFetch(t, app, channelID, ts)
	if got := previewImageBounds(t, out); got.Dx() != 960 || got.Dy() != 720 {
		t.Fatalf("first open returned %v, want 960x720 — the 4:3 original fitted to the budget", got)
	}
	if got := srv.hitCount("/original"); got != 1 {
		t.Fatalf("first open requested /original %d times, want 1", got)
	}
	if got := srv.hitCount("/thumb"); got != 0 {
		t.Fatalf("first open requested /thumb %d times, want 0 — the original covered it", got)
	}

	// Shrink the terminal so the 720 thumb covers the budget.
	app.width, app.height = 80, 40

	out = runPreviewFetch(t, app, channelID, ts)
	if got := srv.hitCount("/thumb"); got != 1 {
		t.Errorf("second open requested /thumb %d times, want 1 — a shared cache key would serve the original's bytes instead", got)
	}
	if got := previewImageBounds(t, out); got.Dx() != 640 || got.Dy() != 640 {
		t.Errorf("second open returned %v, want 640x640 — the square thumbnail, not the 4:3 original", got)
	}
}

// A transient failure on the original must fall back to the thumbnail
// without being recorded as permanent: only an undecodable format earns
// that. Otherwise one bad response would demote a file to thumbnails
// for the rest of the session.
func TestPreviewFetch_TransientOriginalFailureRetriedNextOpen(t *testing.T) {
	srv := newPreviewServer(t, nil, "image/png")
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srv.mu.Lock()
		srv.hits[r.URL.Path]++
		n := srv.hits["/original"]
		srv.mu.Unlock()
		if r.URL.Path == "/original" && n == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Path == "/original" {
			w.Write(makeTestPNGBytes(1600, 1200))
			return
		}
		w.Write(makeTestPNGBytes(720, 720))
	})
	app, channelID, ts := previewTestApp(t, srv, 1600, 1200)

	out := runPreviewFetch(t, app, channelID, ts)
	if got := previewImageBounds(t, out); got.Dx() != 720 || got.Dy() != 720 {
		t.Fatalf("first open returned %v, want the 720 thumbnail untouched (it already fits the budget)", got)
	}

	out = runPreviewFetch(t, app, channelID, ts)
	if got := srv.hitCount("/original"); got != 2 {
		t.Errorf("/original requested %d times total, want 2 — an HTTP 500 is not a dead format", got)
	}
	if got := previewImageBounds(t, out); got.Dx() != 960 || got.Dy() != 720 {
		t.Errorf("second open returned %v, want 960x720 — the original, now that it answers", got)
	}
}

// The budget is the terminal's pixel size, so a small window keeps the
// preview on thumbnails and never spends the original's bandwidth.
func TestPreviewFetch_SmallTerminalNeverAsksForOriginal(t *testing.T) {
	srv := newPreviewServer(t, makeTestPNGBytes(1600, 1200), "image/png")
	app, channelID, ts := previewTestApp(t, srv, 1600, 1200)
	app.width, app.height = 80, 40 // 640x640, inside the 720 thumb

	out := runPreviewFetch(t, app, channelID, ts)
	if got := previewImageBounds(t, out); got.Dx() != 640 || got.Dy() != 640 {
		t.Errorf("got %v, want 640x640 from the thumbnail", got)
	}
	if got := srv.hitCount("/original"); got != 0 {
		t.Errorf("/original requested %d times, want 0", got)
	}
}
