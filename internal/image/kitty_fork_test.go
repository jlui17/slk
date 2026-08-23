package image

import (
	"bytes"
	"compress/zlib"
	"image"
	"io"
	"strings"
	"testing"
)

func TestKitty_UploadRGBAFormat(t *testing.T) {
	t.Setenv("TMUX", "")
	var buf bytes.Buffer
	if err := emitKittyUpload(&buf, 42, "abcd", 10, 5, 170, 185, true); err != nil {
		t.Fatal(err)
	}
	s := buf.String()
	if !strings.Contains(s, "f=32,o=z,s=170,v=185") {
		t.Fatalf("expected raw RGBA header with pixel dims, got %q", s)
	}
	if strings.Contains(s, "f=100") {
		t.Fatalf("PNG format leaked into RGBA-mode upload: %q", s)
	}
}

func TestKitty_EncodePayloadRGBARoundtrip(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	for i := range img.Pix {
		img.Pix[i] = byte(i * 7)
	}
	raw, err := encodeKittyPayload(img, true)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zlib.NewReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("payload is not zlib: %v", err)
	}
	defer zr.Close()
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decoded, img.Pix) {
		t.Fatalf("decoded pixels differ: got %d bytes, want %d", len(decoded), len(img.Pix))
	}
}
