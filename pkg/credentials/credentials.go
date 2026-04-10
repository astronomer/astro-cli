package credentials

import "sync"

// CurrentCredentials holds the current auth token in memory for the duration of a
// command invocation. It is populated by PersistentPreRunE after credentials
// are resolved from the secure store, and read by API client request editors
// on every outbound request.
//
// It is constructed once in NewRootCmd and passed by pointer to both the API
// clients and CreateRootPersistentPreRunE. There is no global state.
type CurrentCredentials struct {
	mu    sync.RWMutex
	token string
}

// New creates a CurrentCredentials with an initial token value.
func New(token string) *CurrentCredentials {
	return &CurrentCredentials{token: token}
}

// Set stores the token.
func (h *CurrentCredentials) Set(token string) {
	h.mu.Lock()
	h.token = token
	h.mu.Unlock()
}

// Get returns the current token.
func (h *CurrentCredentials) Get() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.token
}
