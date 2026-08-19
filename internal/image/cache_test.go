package image

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCache_PutGet(t *testing.T) {
	dir := t.TempDir()
	c, err := NewCache(dir, 10) // 10 MB cap
	if err != nil {
		t.Fatal(err)
	}

	data := []byte("hello-png-bytes")
	path, err := c.Put("k1", "png", data)
	if err != nil {
		t.Fatal(err)
	}

	got, ok := c.Get("k1")
	if !ok {
		t.Fatal("expected hit")
	}
	if got != path {
		t.Errorf("Get path %q != Put path %q", got, path)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("path does not exist: %v", err)
	}
}

func TestCache_Miss(t *testing.T) {
	c, _ := NewCache(t.TempDir(), 10)
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss")
	}
}

func TestCache_LRUEvictsOldest(t *testing.T) {
	dir := t.TempDir()
	// 1 MB cap; entries ~ 600KB each => 2nd Put fits, 3rd evicts oldest.
	c, _ := NewCache(dir, 1)
	bigA := bytes.Repeat([]byte{'a'}, 600*1024)
	bigB := bytes.Repeat([]byte{'b'}, 600*1024)
	bigC := bytes.Repeat([]byte{'c'}, 600*1024)

	if _, err := c.Put("a", "bin", bigA); err != nil {
		t.Fatal(err)
	}
	// Make 'a' older than 'b' by tweaking mtime.
	older := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, "a.bin"), older, older)

	if _, err := c.Put("b", "bin", bigB); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Put("c", "bin", bigC); err != nil {
		t.Fatal(err)
	}

	if _, ok := c.Get("a"); ok {
		t.Errorf("expected 'a' evicted")
	}
	if _, ok := c.Get("b"); !ok {
		t.Errorf("expected 'b' still present")
	}
	if _, ok := c.Get("c"); !ok {
		t.Errorf("expected 'c' present")
	}
}

func TestCache_OversizeEntryAllowed(t *testing.T) {
	c, _ := NewCache(t.TempDir(), 1) // 1 MB cap
	huge := bytes.Repeat([]byte{'x'}, 2*1024*1024)
	if _, err := c.Put("huge", "bin", huge); err != nil {
		t.Fatalf("oversize Put should succeed: %v", err)
	}
	if _, ok := c.Get("huge"); !ok {
		t.Error("expected oversize entry served from cache for this session")
	}
}

func TestCache_GetUpdatesATime(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCache(dir, 10)
	c.Put("k", "bin", []byte("x"))

	older := time.Now().Add(-time.Hour)
	path := filepath.Join(dir, "k.bin")
	os.Chtimes(path, older, older)

	c.Get("k")

	st, _ := os.Stat(path)
	if time.Since(st.ModTime()) > time.Minute {
		t.Errorf("Get should refresh mtime, got %v old", time.Since(st.ModTime()))
	}
}

func TestCache_LoadIndexAtStartup(t *testing.T) {
	dir := t.TempDir()
	c1, _ := NewCache(dir, 10)
	c1.Put("preexisting", "bin", []byte("data"))

	// New cache instance: should pick up the existing file.
	c2, err := NewCache(dir, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c2.Get("preexisting"); !ok {
		t.Error("expected pre-existing entry to be indexed at startup")
	}
}

// A second slk instance shares the cache directory and evicts from its
// own index, so it can delete a file this instance still has indexed.
// Get must report that as a miss and forget the entry, not hand back a
// path the caller cannot open.
func TestCache_GetMissesWhenAnotherProcessDeletedTheFile(t *testing.T) {
	dir := t.TempDir()
	c, _ := NewCache(dir, 10)
	if _, err := c.Put("k", "bin", []byte("some-bytes")); err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(filepath.Join(dir, "k.bin")); err != nil {
		t.Fatalf("remove behind the index: %v", err)
	}

	if _, ok := c.Get("k"); ok {
		t.Fatal("Get returned a hit for a file that is gone")
	}
	entries, total := c.Stats()
	if entries != 0 || total != 0 {
		t.Errorf("Stats = (%d entries, %d bytes) after the miss; want (0, 0)", entries, total)
	}
	// The stale entry is gone, so the next Put re-fills the key.
	if _, err := c.Put("k", "bin", []byte("fresh")); err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Get("k"); !ok {
		t.Error("expected a hit after re-Put")
	}
}

// Eviction of an entry another process already deleted still frees its
// bytes from the accounting; otherwise the cache would keep shrinking
// its usable capacity.
func TestCache_EvictionToleratesAMissingFile(t *testing.T) {
	dir := t.TempDir()
	// 1 MB cap, entries ~600KB: the third Put's pre-sweep evicts "a".
	c, _ := NewCache(dir, 1)
	big := bytes.Repeat([]byte{'a'}, 600*1024)
	for _, key := range []string{"a", "b"} {
		if _, err := c.Put(key, "bin", big); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(dir, "a.bin")); err != nil {
		t.Fatalf("remove behind the index: %v", err)
	}

	if _, err := c.Put("c", "bin", big); err != nil {
		t.Fatal(err)
	}
	entries, total := c.Stats()
	if entries != 2 || total != int64(2*len(big)) {
		t.Errorf("Stats = (%d entries, %d bytes); want (2, %d)", entries, total, 2*len(big))
	}
	if _, ok := c.Get("a"); ok {
		t.Error("evicted entry is still a hit")
	}
}

// A second slk instance rendering the same message Puts the same key,
// so a reader can be opening a cached file exactly as it is rewritten.
// A truncate-then-write hands the decoder a partial image; the rename
// keeps every read whole.
func TestCache_PutNeverExposesAPartialFile(t *testing.T) {
	c, _ := NewCache(t.TempDir(), 10)
	data := bytes.Repeat([]byte{'a'}, 256*1024)
	if _, err := c.Put("k", "bin", data); err != nil {
		t.Fatalf("seed Put: %v", err)
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	putErr := make(chan error, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for !stop.Load() {
			if _, err := c.Put("k", "bin", data); err != nil {
				putErr <- err
				return
			}
		}
	}()

	fail := func(format string, args ...any) {
		stop.Store(true)
		wg.Wait()
		t.Fatalf(format, args...)
	}
	for i := 0; i < 300; i++ {
		path, ok := c.Get("k")
		if !ok {
			fail("read %d: cache miss while the entry was being rewritten", i)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			fail("read %d: %v", i, err)
		}
		if len(got) != len(data) {
			fail("read %d saw %d bytes; want the whole %d", i, len(got), len(data))
		}
	}

	stop.Store(true)
	wg.Wait()
	select {
	case err := <-putErr:
		t.Fatalf("Put: %v", err)
	default:
	}
}

// A temp file stranded by a crash is not a cache entry, and nothing
// else would ever remove it: eviction only deletes paths the index
// knows, and the deferred cleanup died with the process. A fresh one
// may belong to a live Put in another instance, so it stays.
func TestCache_LoadIndexReapsOnlyOldStrandedTempFiles(t *testing.T) {
	dir := t.TempDir()
	stranded := filepath.Join(dir, ".tmp-stranded")
	inflight := filepath.Join(dir, ".tmp-inflight")
	for _, p := range []string{stranded, inflight} {
		if err := os.WriteFile(p, bytes.Repeat([]byte{'a'}, 4096), 0600); err != nil {
			t.Fatal(err)
		}
	}
	aged := time.Now().Add(-2 * staleTempAge)
	if err := os.Chtimes(stranded, aged, aged); err != nil {
		t.Fatal(err)
	}

	c, err := NewCache(dir, 10)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(stranded); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stranded temp file survived (stat: %v); its bytes are leaked for good", err)
	}
	if _, err := os.Stat(inflight); err != nil {
		t.Errorf("deleted a temp file another instance may still be writing: %v", err)
	}
	if entries, total := c.Stats(); entries != 0 || total != 0 {
		t.Errorf("Stats = (%d entries, %d bytes); want (0, 0): neither is a cache entry", entries, total)
	}
}
