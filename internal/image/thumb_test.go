package image

import (
	"image"
	"testing"
)

func TestPickThumb_SmallestThatFits(t *testing.T) {
	thumbs := []ThumbSpec{
		{URL: "u-360", W: 360, H: 360},
		{URL: "u-720", W: 720, H: 720},
		{URL: "u-1024", W: 1024, H: 1024},
	}
	// Target 400x400 — should pick 720.
	url, suffix := PickThumb(thumbs, image.Pt(400, 400))
	if url != "u-720" {
		t.Errorf("got %q, want u-720", url)
	}
	if suffix != "720" {
		t.Errorf("suffix got %q, want 720", suffix)
	}
}

func TestPickThumb_FallsBackToLargest(t *testing.T) {
	thumbs := []ThumbSpec{
		{URL: "u-360", W: 360, H: 360},
	}
	url, _ := PickThumb(thumbs, image.Pt(800, 800))
	if url != "u-360" {
		t.Errorf("got %q, want u-360 (largest available)", url)
	}
}

func TestPickThumb_EmptyReturnsEmpty(t *testing.T) {
	url, _ := PickThumb(nil, image.Pt(100, 100))
	if url != "" {
		t.Errorf("expected empty, got %q", url)
	}
}

func TestPickThumb_RequiresBothAxes(t *testing.T) {
	thumbs := []ThumbSpec{
		{URL: "u-wide", W: 1000, H: 100}, // wide enough but too short
		{URL: "u-square", W: 500, H: 500},
	}
	url, _ := PickThumb(thumbs, image.Pt(400, 400))
	if url != "u-square" {
		t.Errorf("got %q, want u-square (only one that fits both axes)", url)
	}
}

// The preview stretches its source over the pane, so a 1024px thumb on
// a 3000px-wide pane renders soft; the original carries the detail.

func TestPickPreviewSource_OriginalWhenBudgetExceedsThumbs(t *testing.T) {
	thumbs := []ThumbSpec{
		{URL: "u-720", W: 720, H: 720},
		{URL: "u-1024", W: 1024, H: 1024},
	}
	original := ThumbSpec{URL: "u-orig", W: 3200, H: 3200}

	url, suffix, target := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-orig" {
		t.Errorf("url got %q, want u-orig", url)
	}
	if suffix != "orig" {
		t.Errorf("suffix got %q, want orig — a thumb-keyed cache entry must not serve original bytes", suffix)
	}
	// 3200x3200 into a 2400x1800 budget is height-bound.
	if target != image.Pt(1800, 1800) {
		t.Errorf("target got %v, want (1800,1800) — the original shrunk to the budget, aspect preserved", target)
	}
}

func TestPickPreviewSource_ThumbWhenItCoversBudget(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 3200, H: 3200}

	url, suffix, target := PickPreviewSource(thumbs, original, image.Pt(800, 600))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — the thumb already covers the budget", url)
	}
	if suffix != "1024" {
		t.Errorf("suffix got %q, want 1024", suffix)
	}
	if target != (image.Point{}) {
		t.Errorf("target got %v, want zero (no resize) for a thumbnail", target)
	}
}

func TestPickPreviewSource_ThumbWhenOriginalDimsUnknown(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig"} // Slack omitted original_w/original_h

	url, _, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — unknown original dims can't be reasoned about", url)
	}
}

func TestPickPreviewSource_ThumbWhenOriginalIsNoBigger(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 900, H: 900}

	url, _, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — fetching a smaller original buys nothing", url)
	}
}

func TestPickPreviewSource_EmptyReturnsEmpty(t *testing.T) {
	url, _, _ := PickPreviewSource(nil, ThumbSpec{}, image.Pt(2400, 1800))
	if url != "" {
		t.Errorf("expected empty, got %q", url)
	}
}
