package ui

import (
	"context"
	"errors"
	"image"

	imgpkg "github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/messages"
)

// previewSource is the fetch plan for the full-screen image preview:
// the picked source, its thumbnail fallback, and the pixel budget both
// are fitted within.
type previewSource struct {
	url, suffix     string
	fbURL, fbSuffix string
	originalURL     string
	budget          image.Point
}

// pickPreviewSource picks the preview's fetch source for att. The
// overlay stretches its source over the whole pane, so it picks
// against the pane's pixel budget rather than against the thumbs; the
// terminal bounds the pane, so its size is a safe over-estimate.
// ok is false when the attachment offers no usable source.
func (a *App) pickPreviewSource(att messages.Attachment) (previewSource, bool) {
	thumbs := make([]imgpkg.ThumbSpec, len(att.Thumbs))
	for i, t := range att.Thumbs {
		thumbs[i] = imgpkg.ThumbSpec{URL: t.URL, W: t.W, H: t.H}
	}
	budget := image.Pt(a.width*a.imageCtx.CellPixels.X, a.height*a.imageCtx.CellPixels.Y)
	original := imgpkg.ThumbSpec{URL: att.DownloadURL, W: att.OriginalW, H: att.OriginalH}
	url, suffix := imgpkg.PickPreviewSource(thumbs, original, budget)
	if url == "" {
		return previewSource{}, false
	}
	// Slack serves thumbnails for formats Go can't decode (HEIC, TIFF),
	// so an original that fails to fetch or decode falls back to what
	// the same call picks with no original in play.
	fbURL, fbSuffix := imgpkg.PickPreviewSource(thumbs, imgpkg.ThumbSpec{}, budget)
	return previewSource{
		url: url, suffix: suffix,
		fbURL: fbURL, fbSuffix: fbSuffix,
		originalURL: original.URL,
		budget:      budget,
	}, true
}

// fetch runs the plan: fetch the picked source, and on failure fall
// back to the thumbnail pick (marking an undecodable original so
// later picks skip it).
func (s previewSource) fetch(ctx context.Context, fetcher *imgpkg.Fetcher, fileID string) (imgpkg.FetchResult, error) {
	res, err := fetcher.Fetch(ctx, imgpkg.FetchRequest{
		Key:       fileID + "-preview-" + s.suffix,
		URL:       s.url,
		FitWithin: s.budget,
	})
	if err != nil {
		if s.url == s.originalURL && errors.Is(err, imgpkg.ErrUndecodable) {
			imgpkg.MarkOriginalUndecodable(s.url)
		}
		if s.fbURL != "" && s.fbURL != s.url {
			res, err = fetcher.Fetch(ctx, imgpkg.FetchRequest{
				Key:       fileID + "-preview-" + s.fbSuffix,
				URL:       s.fbURL,
				FitWithin: s.budget,
			})
		}
	}
	return res, err
}
