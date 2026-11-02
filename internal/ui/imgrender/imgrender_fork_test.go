package imgrender

import (
	"bytes"
	"context"
	"image"
	"image/png"
	"testing"

	imgpkg "github.com/gammons/slk/internal/image"
)

// The fetch worker keeps no decoded image for one it prerendered, so a
// rendered image must come from Prerendered, not wait on Cached: a
// renderer that asked Cached first would draw the loading placeholder
// and fetch again, every render, for an image it already has.
func TestRenderBlock_PrerenderedImageRendersWithoutDecodedCopy(t *testing.T) {
	for _, proto := range []imgpkg.Protocol{imgpkg.ProtoHalfBlock, imgpkg.ProtoKitty} {
		t.Run(proto.String(), func(t *testing.T) {
			cache, err := imgpkg.NewCache(t.TempDir(), 10)
			if err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 720, 720))); err != nil {
				t.Fatal(err)
			}
			const key = "F1-720"
			if _, err := cache.Put(key, "png", buf.Bytes()); err != nil {
				t.Fatal(err)
			}
			fetcher := imgpkg.NewFetcher(cache, nil)
			ctx := ImageContext{Protocol: proto, Fetcher: fetcher, CellPixels: image.Pt(8, 16), MaxRows: 20}
			fetcher.ConfigurePrerender(proto)
			if proto == imgpkg.ProtoKitty {
				ctx.KittyRender = imgpkg.NewKittyRenderer(imgpkg.NewRegistry())
				fetcher.ConfigurePrerenderKitty(ctx.KittyRender)
			}
			// A 720x720 thumb at width 80 renders at 40x20 cells,
			// 320x320 pixels: the fetch the renderer itself would issue.
			if _, err := fetcher.Fetch(context.Background(), imgpkg.FetchRequest{
				Key: key, URL: "unused://disk-cache-hit", Target: image.Pt(320, 320), CellTarget: image.Pt(40, 20),
			}); err != nil {
				t.Fatal(err)
			}
			if _, ok := fetcher.Cached(key, image.Pt(320, 320)); ok {
				t.Fatal("fixture: the decoded memo holds the image, so this would not test the prerendered path")
			}

			r := NewRenderer()
			r.SetContext(ctx)
			res := r.RenderBlock(Block{
				Kind: "image", FileID: "F1", Name: "x.png",
				Thumbs: []ThumbSpec{{URL: "https://example.com/720.png", W: 720, H: 720}},
			}, "C1", "1.0", 80, 0, 0, 0)

			want, _ := fetcher.Prerendered(key, image.Pt(40, 20), proto)
			if len(res.Lines) != 20 || res.Lines[0] != want.Lines[0] {
				t.Errorf("RenderBlock drew %d lines not taken from the prerender", len(res.Lines))
			}
			if len(r.fetching) != 0 {
				t.Errorf("RenderBlock started a fetch for a prerendered image: %v", r.fetching)
			}
		})
	}
}
