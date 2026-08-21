package main

import (
	"testing"
	"time"
)

func TestPreviewCache(t *testing.T) {
	now := time.Unix(1000, 0)
	c := newPreviewCache()
	c.now = func() time.Time { return now }

	if _, _, ok := c.Get("C1", "1.0"); ok {
		t.Fatal("hit on an empty cache")
	}
	c.Put("C1", "1.0", "U1", "hello")
	if u, txt, ok := c.Get("C1", "1.0"); !ok || u != "U1" || txt != "hello" {
		t.Errorf("Get = (%q, %q, %v), want (U1, hello, true)", u, txt, ok)
	}

	c.Put("C1", "2.0", "", "")
	if u, txt, ok := c.Get("C1", "2.0"); !ok || u != "" || txt != "" {
		t.Errorf("not-found entry = (%q, %q, %v), want cached empty hit", u, txt, ok)
	}

	now = now.Add(previewCacheTTL + time.Second)
	if _, _, ok := c.Get("C1", "1.0"); ok {
		t.Error("expired entry served")
	}
}
