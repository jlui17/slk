package sharedmap

import (
	"fmt"
	"sync"
	"testing"
)

func TestMapSemantics(t *testing.T) {
	m := New[string, int64]()
	if len(m.Current()) != 0 {
		t.Fatalf("fresh map Current has %d entries, want 0", len(m.Current()))
	}
	m.Set("C1", 100)
	if v, ok := m.Get("C1"); !ok || v != 100 {
		t.Fatalf("Get(C1) = (%d, %v), want (100, true)", v, ok)
	}
	if _, ok := m.Get("C2"); ok {
		t.Fatal("Get(C2) hit, want miss")
	}

	src := map[string]int64{"C1": 1, "C2": 2}
	seeded := FromMap(src)
	src["C1"] = 99
	if v, _ := seeded.Get("C1"); v != 1 {
		t.Fatalf("FromMap aliased its input: Get(C1) = %d, want 1", v)
	}
	if v, ok := seeded.Get("C2"); !ok || v != 2 {
		t.Fatalf("Get(C2) = (%d, %v), want (2, true)", v, ok)
	}

	snapshot := seeded.Current()
	seeded.Set("C3", 3)
	if _, ok := snapshot["C3"]; ok {
		t.Fatal("a Set mutated a previously returned snapshot")
	}

	var nilMap *Map[string, int64]
	if v, ok := nilMap.Get("C1"); ok || v != 0 {
		t.Fatalf("nil map Get = (%d, %v), want (0, false)", v, ok)
	}
	if nilMap.Current() != nil {
		t.Fatal("nil map Current != nil")
	}
	nilMap.Set("C1", 1)
}

// TestMapConcurrentSetAndCurrent guards the map's own mechanism — the
// atomic publish — against a fatal concurrent-map throw. Only
// meaningful under -race; do not delete as slow.
func TestMapConcurrentSetAndCurrent(t *testing.T) {
	m := New[string, int64]()
	const n = 500
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			m.Set(fmt.Sprintf("A%04d", i), int64(i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			m.Set(fmt.Sprintf("B%04d", i), int64(i))
		}
	}()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for {
		select {
		case <-done:
			if got := len(m.Current()); got != 2*n {
				t.Fatalf("len(Current) = %d, want %d", got, 2*n)
			}
			return
		default:
		}
		current := m.Current()
		for id := range current {
			_ = current[id]
		}
		_, _ = m.Get("A0000")
	}
}
