package secrets

import (
	"errors"
	"testing"
)

func TestMemoryStore(t *testing.T) {
	s := NewMemoryStore()

	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
	if err := s.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}

	if err := s.Set("a", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Set("b", "2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if v, err := s.Get("a"); err != nil || v != "1" {
		t.Fatalf("Get(a) = %q, %v; want %q, nil", v, err, "1")
	}

	metas, err := s.ListMeta()
	if err != nil {
		t.Fatalf("ListMeta: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("ListMeta returned %d entries, want 2", len(metas))
	}
	for _, m := range metas {
		if m.Key != "a" && m.Key != "b" {
			t.Fatalf("ListMeta returned unexpected key %q", m.Key)
		}
	}

	if err := s.Delete("a"); err != nil {
		t.Fatalf("Delete(a): %v", err)
	}
	if _, err := s.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(a) after delete = %v, want ErrNotFound", err)
	}
}
