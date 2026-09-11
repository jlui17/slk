package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang.design/x/clipboard"
)

func TestRemoteClipboardReader(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nfake")
	tests := []struct {
		name   string
		status int
		body   []byte
		format clipboard.Format
		want   []byte
	}{
		{"image 200 returns bytes", http.StatusOK, png, clipboard.FmtImage, png},
		{"text 200 returns text", http.StatusOK, []byte("hello"), clipboard.FmtText, []byte("hello")},
		{"204 returns nil", http.StatusNoContent, nil, clipboard.FmtImage, nil},
		{"500 returns nil", http.StatusInternalServerError, []byte("boom"), clipboard.FmtText, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				w.Write(tt.body)
			}))
			defer srv.Close()
			read := RemoteClipboardReader(strings.TrimPrefix(srv.URL, "http://"))
			if got := read(tt.format); !bytes.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRemoteClipboardReader_RoutesFormatToPath(t *testing.T) {
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	read := RemoteClipboardReader(strings.TrimPrefix(srv.URL, "http://"))
	read(clipboard.FmtImage)
	read(clipboard.FmtText)
	if got := strings.Join(paths, ","); got != "/image,/text" {
		t.Errorf("paths = %q, want /image,/text", got)
	}
}

func TestRemoteClipboardReader_UnreachableReturnsNil(t *testing.T) {
	// A server closed before the read leaves a port nothing listens on.
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := strings.TrimPrefix(srv.URL, "http://")
	srv.Close()
	if got := RemoteClipboardReader(addr)(clipboard.FmtImage); got != nil {
		t.Errorf("got %q, want nil", got)
	}
}
