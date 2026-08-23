package image

import (
	"context"
	"errors"
	"image"
	imgcolor "image/color"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Slack reports original_w/original_h after applying EXIF orientation
// while Go's JPEG decoder ignores it, so a caller sizing the fetch from
// Slack's metadata would ask for a rectangle turned 90° from the bytes.
// FitWithin takes its scale from the decoded bounds so that disagreement
// can only pick a source, never squash the picture.
func TestFetcher_FitWithinPreservesDecodedAspect(t *testing.T) {
	pngBytes := tinyPNG(t, 40, 30, imgcolor.RGBA{10, 20, 30, 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	cache, _ := NewCache(t.TempDir(), 10)
	f := NewFetcher(cache, http.DefaultClient)

	// A square box: a naive resize-to-target would return 20x20.
	res, err := f.Fetch(context.Background(), FetchRequest{
		Key: "landscape", URL: srv.URL, FitWithin: image.Pt(20, 20),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Img.Bounds(); got.Dx() != 20 || got.Dy() != 15 {
		t.Errorf("got %v, want 20x15 — the decoded 4:3 aspect must survive the fit", got)
	}
}

func TestFetcher_FitWithinLeavesSmallImagesAlone(t *testing.T) {
	pngBytes := tinyPNG(t, 40, 30, imgcolor.RGBA{10, 20, 30, 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	cache, _ := NewCache(t.TempDir(), 10)
	f := NewFetcher(cache, http.DefaultClient)

	res, err := f.Fetch(context.Background(), FetchRequest{
		Key: "small", URL: srv.URL, FitWithin: image.Pt(500, 500),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Img.Bounds(); got.Dx() != 40 || got.Dy() != 30 {
		t.Errorf("got %v, want the untouched 40x30 — an image inside the box must not be upscaled", got)
	}
}

// Callers need to tell "these bytes will never decode" apart from "the
// network had a bad moment", so the decode failure carries a sentinel.
func TestFetcher_UndecodableBytesCarrySentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/heic")
		w.Write([]byte("ftypheic not a format Go can read"))
	}))
	defer srv.Close()

	cache, _ := NewCache(t.TempDir(), 10)
	f := NewFetcher(cache, http.DefaultClient)

	_, err := f.Fetch(context.Background(), FetchRequest{Key: "heic", URL: srv.URL})
	if err == nil {
		t.Fatal("expected a decode error for undecodable bytes")
	}
	if !errors.Is(err, ErrUndecodable) {
		t.Errorf("err = %v, want it to wrap ErrUndecodable", err)
	}
}

// Nothing reads a memo entry from a FitWithin fetch: Cached() is the
// only reader and it looks up inline keys at a nonzero target. Storing
// one would pin a screen-sized RGBA for the life of the process.
func TestFetcher_FitWithinDoesNotPopulateDecodedMemo(t *testing.T) {
	pngBytes := tinyPNG(t, 40, 30, imgcolor.RGBA{10, 20, 30, 255})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	cache, _ := NewCache(t.TempDir(), 10)
	f := NewFetcher(cache, http.DefaultClient)

	if _, err := f.Fetch(context.Background(), FetchRequest{
		Key: "preview", URL: srv.URL, FitWithin: image.Pt(20, 20),
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Cached("preview", image.Point{}); ok {
		t.Error("a FitWithin fetch left an entry in the decoded memo; nothing ever reads it back")
	}

	// A Target fetch still memoizes — that entry has a real reader.
	if _, err := f.Fetch(context.Background(), FetchRequest{
		Key: "inline", URL: srv.URL, Target: image.Pt(20, 15),
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.Cached("inline", image.Pt(20, 15)); !ok {
		t.Error("a Target fetch must still populate the memo Cached() serves from")
	}
}
