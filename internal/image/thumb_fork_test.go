package image

import (
	"image"
	"testing"
)

// The preview stretches its source over the pane, so a 1024px thumb on
// a 3000px-wide pane renders soft; the original carries the detail.

func TestPickPreviewSource_OriginalWhenBudgetExceedsThumbs(t *testing.T) {
	thumbs := []ThumbSpec{
		{URL: "u-720", W: 720, H: 720},
		{URL: "u-1024", W: 1024, H: 1024},
	}
	original := ThumbSpec{URL: "u-orig", W: 3200, H: 3200}

	url, suffix := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-orig" {
		t.Errorf("url got %q, want u-orig", url)
	}
	if suffix != "orig" {
		t.Errorf("suffix got %q, want orig — a thumb-keyed cache entry must not serve original bytes", suffix)
	}
}

func TestPickPreviewSource_ThumbWhenItCoversBudget(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 3200, H: 3200}

	url, suffix := PickPreviewSource(thumbs, original, image.Pt(800, 600))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — the thumb already covers the budget", url)
	}
	if suffix != "1024" {
		t.Errorf("suffix got %q, want 1024", suffix)
	}
}

func TestPickPreviewSource_ThumbWhenOriginalDimsUnknown(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig"} // Slack omitted original_w/original_h

	url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — unknown original dims can't be reasoned about", url)
	}
}

func TestPickPreviewSource_ThumbWhenOriginalIsNoBigger(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 900, H: 900}

	url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — fetching a smaller original buys nothing", url)
	}
}

func TestPickPreviewSource_EmptyReturnsEmpty(t *testing.T) {
	url, _ := PickPreviewSource(nil, ThumbSpec{}, image.Pt(2400, 1800))
	if url != "" {
		t.Errorf("expected empty, got %q", url)
	}
}

// Decoding allocates the whole original uncompressed, so a huge upload
// would spike hundreds of megabytes on one keypress. Above the ceiling
// the thumbnail wins even though it renders softer.
func TestPickPreviewSource_RefusesOversizedOriginal(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 12000, H: 12000} // 144MP panorama

	url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — a 144MP original is past the decode ceiling", url)
	}
}

func TestPickPreviewSource_AcceptsOriginalAtCeiling(t *testing.T) {
	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 8000, H: 5000} // 40MP exactly

	url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800))
	if url != "u-orig" {
		t.Errorf("url got %q, want u-orig — 40MP is the ceiling, not past it", url)
	}
}

// A decode failure evicts the cache entry, so nothing on disk remembers
// that a HEIC or TIFF original is unreadable. Without the in-process
// note, every preview open would re-download it in full.
func TestPickPreviewSource_SkipsOriginalKnownUndecodable(t *testing.T) {
	ResetUndecodableOriginalsForTest()
	t.Cleanup(ResetUndecodableOriginalsForTest)

	thumbs := []ThumbSpec{{URL: "u-1024", W: 1024, H: 1024}}
	original := ThumbSpec{URL: "u-orig", W: 3200, H: 3200}

	if url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800)); url != "u-orig" {
		t.Fatalf("precondition: url got %q, want u-orig before the failure is recorded", url)
	}

	MarkOriginalUndecodable("u-orig")

	if url, _ := PickPreviewSource(thumbs, original, image.Pt(2400, 1800)); url != "u-1024" {
		t.Errorf("url got %q, want u-1024 — a recorded decode failure must not be retried", url)
	}
}
