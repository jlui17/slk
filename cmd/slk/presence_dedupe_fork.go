package main

import "sync"

type presenceDedupe struct {
	mu   sync.Mutex
	last map[string]string
}

func (d *presenceDedupe) Changed(userID, presence string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if last, ok := d.last[userID]; ok && last == presence {
		return false
	}
	if d.last == nil {
		d.last = make(map[string]string)
	}
	d.last[userID] = presence
	return true
}
