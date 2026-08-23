package image

import (
	"os"
	"path/filepath"
	"time"
)

// staleTempAge is how old a leftover temp file must be before a
// starting cache deletes it. A Put writes its bytes and renames in
// milliseconds, so a temp file this old was stranded by a crash rather
// than being written right now by another instance. Lower it far enough
// and a boot deletes a live Put's file out from under it; raising it
// only delays reclaiming the space.
const staleTempAge = 10 * time.Minute

// writeCacheFile writes data to a temp file in path's directory and
// renames it over path, so a reader gets either the whole old entry or
// the whole new one.
//
// Entries are content-addressed but the directory is not private to one
// process: two slk instances rendering the same message Put the same
// key at the same moment, and with a plain os.WriteFile one truncates
// the file while the other's decoder is reading it.
//
// No fsync, unlike the config saver: a torn file surviving a crash
// fails to decode, and the fetcher already deletes an entry that fails
// to decode and re-downloads it. That is cheaper than an fsync on every
// image slk caches.
func writeCacheFile(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)

	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// os.CreateTemp creates the file 0600, the mode entries already had.
	return os.Rename(tmp, path)
}

// dropLocked removes it from the index, the LRU list and disk, keeping
// the byte total in step. A file another process already deleted is not
// an error: the accounting has to come off either way, or this cache
// hoards capacity for bytes that are gone. Caller must hold c.mu.
func (c *Cache) dropLocked(it *item) {
	c.lru.Remove(it.elem)
	delete(c.items, it.key)
	c.total -= it.size
	_ = os.Remove(it.path)
}
