package demo

import (
	"image"
	"testing"
)

func TestMemeDecodesAtItsDeclaredSize(t *testing.T) {
	img, err := decodeMeme()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.Bounds().Size(), image.Pt(memeW, memeH); got != want {
		t.Fatalf("meme is %v, want %v", got, want)
	}
}

func TestMemeAttachmentIsAnInlineImage(t *testing.T) {
	a := memeAttachment()
	if a.Kind != "image" || a.FileID != memeFileID || a.Mime != "image/jpeg" {
		t.Fatalf("attachment = %+v", a)
	}
	if len(a.Thumbs) != 1 || a.Thumbs[0].W != memeW || a.Thumbs[0].H != memeH || a.Thumbs[0].URL == "" {
		t.Fatalf("thumbs = %+v, want one %dx%d thumb with a URL", a.Thumbs, memeW, memeH)
	}
}
