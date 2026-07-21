package secrets

import "sync"

// NewMemoryStore returns an in-memory Store for tests.
func NewMemoryStore() Store {
	return &memoryStore{values: map[string]string{}}
}

type memoryStore struct {
	mu     sync.Mutex
	values map[string]string
}

func (m *memoryStore) Get(key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.values[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *memoryStore) Set(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = value
	return nil
}

func (m *memoryStore) Delete(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.values[key]; !ok {
		return ErrNotFound
	}
	delete(m.values, key)
	return nil
}

func (m *memoryStore) ListMeta() ([]Meta, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	metas := make([]Meta, 0, len(m.values))
	for k := range m.values {
		metas = append(metas, Meta{Key: k})
	}
	return metas, nil
}
