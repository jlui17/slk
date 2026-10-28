package avatar

import (
	"bytes"
	"image"
	imgcolor "image/color"
	imgpng "image/png"
	"io"
	"net/http"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// pngTransport serves a fixed PNG for every request, so tests run the
// avatar pipeline without real network.
type pngTransport struct {
	png []byte
}

func (rt *pngTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     http.Header{"Content-Type": []string{"image/png"}},
		Body:       io.NopCloser(bytes.NewReader(rt.png)),
		Request:    req,
	}, nil
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

func fetcherWithTransport(t *testing.T, rt *pngTransport) *imgpkg.Fetcher {
	t.Helper()
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	return imgpkg.NewFetcher(cache, &http.Client{Transport: rt})
}

// TestPreloadSync_KittyDecodeIsBounded asserts the kitty path no longer
// retains the full-resolution decode: a 512x512 source must be memoized
// at the bounded 128x128 target, and no zero-target (full-res) entry
// may exist.
func TestPreloadSync_KittyDecodeIsBounded(t *testing.T) {
	t.Setenv("TMUX", "")
	rt := &pngTransport{png: pngBytesOfSize(t, 512)}
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
	img, ok := fetcher.Cached(fetchKey("U_BOUND", "https://example.com/big.png"), image.Pt(128, 128))
	if !ok {
		t.Fatal("decoded kitty avatar not memoized at bounded 128x128 target")
	}
	b := img.Bounds()
	if b.Dx() != 128 || b.Dy() != 128 {
		t.Errorf("bounded decode is %dx%d, want 128x128", b.Dx(), b.Dy())
	}
	if _, full := fetcher.Cached(fetchKey("U_BOUND", "https://example.com/big.png"), image.Point{}); full {
		t.Error("full-resolution decode retained at zero target; kitty avatars must be bounded")
	}
}

// TestPreloadSync_HalfblockDecodeTargetUnchanged guards the half-block
// decode target at (AvatarCols, AvatarRows*2): the parity test depends
// on byte-identical half-block output, so the kitty bound must not
// leak into this path.
func TestPreloadSync_HalfblockDecodeTargetUnchanged(t *testing.T) {
	rt := &pngTransport{png: pngBytesOfSize(t, 512)}
	fetcher := fetcherWithTransport(t, rt)
	c := NewCache(fetcher, nil, false)

	c.PreloadSync("U_HB", "https://example.com/big.png")

	img, ok := fetcher.Cached(fetchKey("U_HB", "https://example.com/big.png"), image.Pt(AvatarCols, AvatarRows*2))
	if !ok {
		t.Fatal("half-block avatar not memoized at (AvatarCols, AvatarRows*2)")
	}
	b := img.Bounds()
	if b.Dx() != AvatarCols || b.Dy() != AvatarRows*2 {
		t.Errorf("half-block decode is %dx%d, want %dx%d", b.Dx(), b.Dy(), AvatarCols, AvatarRows*2)
	}
}
