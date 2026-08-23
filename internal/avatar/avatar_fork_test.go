package avatar

import (
	"bytes"
	"image"
	imgcolor "image/color"
	imgpng "image/png"
	"io"
	"net/http"
	"sync"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// recordingTransport serves a fixed PNG for every request and records
// the URLs it was asked for, so tests can observe the exact URL the
// avatar pipeline fetches without real network.
type recordingTransport struct {
	mu   sync.Mutex
	urls []string
	png  []byte
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt.mu.Lock()
	rt.urls = append(rt.urls, req.URL.String())
	rt.mu.Unlock()
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(rt.png)),
		Request:    req,
	}, nil
}

func (rt *recordingTransport) requested() []string {
	rt.mu.Lock()
	defer rt.mu.Unlock()
	return append([]string(nil), rt.urls...)
}

func pngBytesOfSize(t *testing.T, size int) []byte {
	t.Helper()
	src := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y += 8 {
		for x := 0; x < size; x += 8 {
			src.Set(x, y, imgcolor.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := imgpng.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func fetcherWithTransport(t *testing.T, rt *recordingTransport) *imgpkg.Fetcher {
	t.Helper()
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return imgpkg.NewFetcher(cache, &http.Client{Transport: rt})
}

func TestSizedAvatarURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "original jpg rewritten",
			in:   "https://avatars.slack-edge.com/2024-06-01/1234567890_abcdef_original.jpg",
			want: "https://avatars.slack-edge.com/2024-06-01/1234567890_abcdef_192.jpg",
		},
		{
			name: "original png rewritten",
			in:   "https://avatars.slack-edge.com/2023-11-12/999_deadbeef_original.png",
			want: "https://avatars.slack-edge.com/2023-11-12/999_deadbeef_192.png",
		},
		{
			name: "original gif rewritten",
			in:   "https://avatars.slack-edge.com/2023-11-12/999_deadbeef_original.gif",
			want: "https://avatars.slack-edge.com/2023-11-12/999_deadbeef_192.gif",
		},
		{
			name: "already sized variant untouched",
			in:   "https://avatars.slack-edge.com/2024-06-01/1234567890_abcdef_512.jpg",
			want: "https://avatars.slack-edge.com/2024-06-01/1234567890_abcdef_512.jpg",
		},
		{
			name: "gravatar untouched",
			in:   "https://secure.gravatar.com/avatar/abc123.jpg?s=192&d=https%3A%2F%2Fa.slack-edge.com%2Fdf10d%2Fimg%2Favatars%2Fava_0011-192.png",
			want: "https://secure.gravatar.com/avatar/abc123.jpg?s=192&d=https%3A%2F%2Fa.slack-edge.com%2Fdf10d%2Fimg%2Favatars%2Fava_0011-192.png",
		},
		{
			name: "non-slack-edge host with original marker untouched",
			in:   "https://example.com/2024-06-01/1234_abcd_original.jpg",
			want: "https://example.com/2024-06-01/1234_abcd_original.jpg",
		},
		{
			name: "original marker mid-path untouched",
			in:   "https://avatars.slack-edge.com/2024_original.dir/file.jpg",
			want: "https://avatars.slack-edge.com/2024_original.dir/file.jpg",
		},
		{
			name: "no extension after marker untouched",
			in:   "https://avatars.slack-edge.com/2024-06-01/1234_abcd_original.",
			want: "https://avatars.slack-edge.com/2024-06-01/1234_abcd_original.",
		},
		{
			name: "query string after extension untouched",
			in:   "https://avatars.slack-edge.com/2024-06-01/1234_abcd_original.jpg?x=1",
			want: "https://avatars.slack-edge.com/2024-06-01/1234_abcd_original.jpg?x=1",
		},
		{
			name: "empty untouched",
			in:   "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sizedAvatarURL(tc.in); got != tc.want {
				t.Errorf("sizedAvatarURL(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestPreloadSync_FetchesSizedVariantForOriginalURL asserts the URL
// rewrite is wired into the preload path: an _original slack-edge URL
// must reach the HTTP layer as the _192 variant.
func TestPreloadSync_FetchesSizedVariantForOriginalURL(t *testing.T) {
	rt := &recordingTransport{png: pngBytesOfSize(t, 512)}
	fetcher := fetcherWithTransport(t, rt)
	c := NewCache(fetcher, nil, false)

	c.PreloadSync("U_RW", "https://avatars.slack-edge.com/2024-06-01/1234_abcd_original.png")

	got := rt.requested()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 fetch, got %d: %v", len(got), got)
	}
	want := "https://avatars.slack-edge.com/2024-06-01/1234_abcd_192.png"
	if got[0] != want {
		t.Errorf("fetched %q, want %q", got[0], want)
	}
}

// TestPreloadSync_KittyDecodeIsBounded asserts the kitty path no longer
// retains the full-resolution decode: a 512x512 source must be memoized
// at the bounded 128x128 target, and no zero-target (full-res) entry
// may exist.
func TestPreloadSync_KittyDecodeIsBounded(t *testing.T) {
	t.Setenv("TMUX", "")
	rt := &recordingTransport{png: pngBytesOfSize(t, 512)}
	fetcher := fetcherWithTransport(t, rt)

	saved := imgpkg.KittyOutput
	defer func() { imgpkg.KittyOutput = saved }()
	var sideCh bytes.Buffer
	imgpkg.KittyOutput = &sideCh

	kitty := imgpkg.NewKittyRenderer(imgpkg.NewRegistry())
	c := NewCache(fetcher, kitty, true)

	c.PreloadSync("U_BOUND", "https://example.com/big.png")

	if c.Get("U_BOUND") == "" {
		t.Fatal("expected non-empty kitty avatar render")
	}
	img, ok := fetcher.Cached("avatar-U_BOUND", image.Pt(128, 128))
	if !ok {
		t.Fatal("decoded kitty avatar not memoized at bounded 128x128 target")
	}
	b := img.Bounds()
	if b.Dx() != 128 || b.Dy() != 128 {
		t.Errorf("bounded decode is %dx%d, want 128x128", b.Dx(), b.Dy())
	}
	if _, full := fetcher.Cached("avatar-U_BOUND", image.Point{}); full {
		t.Error("full-resolution decode retained at zero target; kitty avatars must be bounded")
	}
}

// TestPreloadSync_HalfblockDecodeTargetUnchanged guards the half-block
// decode target at (AvatarCols, AvatarRows*2): the parity test depends
// on byte-identical half-block output, so the kitty bound must not
// leak into this path.
func TestPreloadSync_HalfblockDecodeTargetUnchanged(t *testing.T) {
	rt := &recordingTransport{png: pngBytesOfSize(t, 512)}
	fetcher := fetcherWithTransport(t, rt)
	c := NewCache(fetcher, nil, false)

	c.PreloadSync("U_HB", "https://example.com/big.png")

	img, ok := fetcher.Cached("avatar-U_HB", image.Pt(AvatarCols, AvatarRows*2))
	if !ok {
		t.Fatal("half-block avatar not memoized at (AvatarCols, AvatarRows*2)")
	}
	b := img.Bounds()
	if b.Dx() != AvatarCols || b.Dy() != AvatarRows*2 {
		t.Errorf("half-block decode is %dx%d, want %dx%d", b.Dx(), b.Dy(), AvatarCols, AvatarRows*2)
	}
}
