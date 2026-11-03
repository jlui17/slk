package kittytest

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// Fetcher returns a fetcher that has already fetched and kitty-prerendered
// each key at cells, as the fetch worker does before a pane draws an
// image. cellPx is the pane's cell size; the image's pixel target is
// cells × cellPx.
func Fetcher(t testing.TB, kr *imgpkg.KittyRenderer, keys []string, cells, cellPx image.Point) *imgpkg.Fetcher {
	t.Helper()
	cache, err := imgpkg.NewCache(t.TempDir(), 10)
	if err != nil {
		t.Fatal(err)
	}
	src := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range src.Pix {
		src.Pix[i] = uint8(i)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatal(err)
	}
	f := imgpkg.NewFetcher(cache, nil)
	f.ConfigurePrerender(imgpkg.ProtoKitty)
	f.ConfigurePrerenderKitty(kr)
	for _, key := range keys {
		if _, err := cache.Put(key, "png", buf.Bytes()); err != nil {
			t.Fatal(err)
		}
		if _, err := f.Fetch(context.Background(), imgpkg.FetchRequest{
			Key:        key,
			URL:        "unused://disk-cache-hit",
			Target:     image.Pt(cells.X*cellPx.X, cells.Y*cellPx.Y),
			CellTarget: cells,
		}); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.Prerendered(key, cells, imgpkg.ProtoKitty); !ok {
			t.Fatalf("%s: not prerendered", key)
		}
	}
	return f
}
