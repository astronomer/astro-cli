package keychain

import "sync"

// cachedStore wraps a SecureStore and caches credentials in memory so that
// repeated reads of the same domain within a single process only hit the
// underlying store (and trigger an OS keychain prompt) once.
type cachedStore struct {
	inner SecureStore
	mu    sync.RWMutex
	cache map[string]Credentials
}

func newCachedStore(inner SecureStore) SecureStore {
	return &cachedStore{inner: inner, cache: make(map[string]Credentials)}
}

func (c *cachedStore) GetCredentials(domain string) (Credentials, error) {
	c.mu.RLock()
	creds, ok := c.cache[domain]
	c.mu.RUnlock()
	if ok {
		return creds, nil
	}

	creds, err := c.inner.GetCredentials(domain)
	if err != nil {
		return Credentials{}, err
	}

	c.mu.Lock()
	c.cache[domain] = creds
	c.mu.Unlock()
	return creds, nil
}

func (c *cachedStore) SetCredentials(domain string, creds Credentials) error {
	if err := c.inner.SetCredentials(domain, creds); err != nil {
		return err
	}
	c.mu.Lock()
	c.cache[domain] = creds
	c.mu.Unlock()
	return nil
}

func (c *cachedStore) DeleteCredentials(domain string) error {
	if err := c.inner.DeleteCredentials(domain); err != nil {
		return err
	}
	c.mu.Lock()
	delete(c.cache, domain)
	c.mu.Unlock()
	return nil
}
