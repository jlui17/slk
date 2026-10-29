package image

// kittyStoreBudgetBytes is how many decoded bytes of uploads slk trusts a
// terminal to keep. herdr (0.9.x) holds 64 MiB of decoded pixels per pane
// screen (KITTY_IMAGE_STORAGE_LIMIT_BYTES in its crates/ghostty-vt) and,
// once a new upload passes that, deletes the oldest uploads first, even
// ones still on screen; Ghostty and kitty do the same at 320 MB. The
// margin below 64 MiB covers whatever else the store holds.
const kittyStoreBudgetBytes = 48 << 20

// markSent records that id was uploaded with decodedBytes of pixels, at
// the end of the running upload total.
func (r *Registry) markSent(id uint32, decodedBytes int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sentAt == nil {
		r.sentAt = map[uint32]uint64{}
	}
	r.sentAt[id] = r.sent
	r.sent += uint64(decodedBytes)
	r.uploaded[id] = true
}

// evicted reports whether more than kittyStoreBudgetBytes went out from
// id's last upload on, so the terminal may have dropped it. Re-sending an
// image counts it again, so the total runs ahead of what the terminal
// holds and an image is re-sent early rather than late.
func (r *Registry) evicted(id uint32) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.evictedLocked(id)
}

func (r *Registry) evictedLocked(id uint32) bool {
	at, ok := r.sentAt[id]
	return ok && r.sent-at > kittyStoreBudgetBytes
}
