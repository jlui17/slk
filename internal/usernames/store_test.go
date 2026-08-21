package usernames

import (
	"fmt"
	"sync"
	"testing"
)

func TestStoreSemantics(t *testing.T) {
	s := NewStore()
	if v := s.Version(); v != 0 {
		t.Fatalf("fresh store Version = %d, want 0", v)
	}
	s.Set("U1", "Alice")
	if name, ok := s.Get("U1"); !ok || name != "Alice" {
		t.Fatalf("Get(U1) = (%q, %v), want (\"Alice\", true)", name, ok)
	}
	if v := s.Version(); v != 1 {
		t.Fatalf("Version after Set = %d, want 1", v)
	}
	s.Set("U1", "Alice") // redundant: no publish, no version bump
	if v := s.Version(); v != 1 {
		t.Fatalf("Version after redundant Set = %d, want 1", v)
	}
	s.Apply(map[string]string{"U1": "Alice", "U2": "Bob"})
	if v := s.Version(); v != 2 {
		t.Fatalf("Version after Apply = %d, want 2", v)
	}
	if s.Len() != 2 {
		t.Fatalf("Len = %d, want 2", s.Len())
	}
	s.Apply(map[string]string{"U1": "Alice", "U2": "Bob"}) // fully redundant batch
	if v := s.Version(); v != 2 {
		t.Fatalf("Version after redundant Apply = %d, want 2", v)
	}

	s.Fill(map[string]string{"U1": "Alicia", "U3": "Cara"})
	if name, _ := s.Get("U1"); name != "Alice" {
		t.Fatalf("Fill overwrote an existing name: Get(U1) = %q, want Alice", name)
	}
	if name, _ := s.Get("U3"); name != "Cara" {
		t.Fatalf("Fill missed a new entry: Get(U3) = %q, want Cara", name)
	}
	if v := s.Version(); v != 3 {
		t.Fatalf("Version after Fill = %d, want 3", v)
	}
	s.Fill(map[string]string{"U1": "Alicia"}) // nothing fillable: no publish
	if v := s.Version(); v != 3 {
		t.Fatalf("Version after no-op Fill = %d, want 3", v)
	}

	var nilStore *Store
	if name, ok := nilStore.Get("U1"); ok || name != "" {
		t.Fatalf("nil store Get = (%q, %v), want (\"\", false)", name, ok)
	}
	nilStore.Set("U1", "x")
	nilStore.Apply(map[string]string{"U1": "x"})
	if nilStore.Len() != 0 || nilStore.Version() != 0 {
		t.Fatalf("nil store Len/Version = %d/%d, want 0/0", nilStore.Len(), nilStore.Version())
	}
}

// TestStoreConcurrentSetAndCurrent guards the store's own mechanism —
// the atomic publish — against a fatal concurrent-map throw. Only
// meaningful under -race; do not delete as slow.
func TestStoreConcurrentSetAndCurrent(t *testing.T) {
	s := NewStore()
	const n = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			s.Set(fmt.Sprintf("U%04d", i), fmt.Sprintf("User %d", i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			s.Apply(map[string]string{
				fmt.Sprintf("B%04d", i): fmt.Sprintf("Batch %d", i),
			})
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for {
		select {
		case <-done:
			if got := s.Len(); got != 2*n {
				t.Fatalf("Len = %d, want %d", got, 2*n)
			}
			return
		default:
		}
		current := s.Current()
		for id := range current {
			_ = current[id]
		}
		_, _ = s.Get("U0000")
	}
}
