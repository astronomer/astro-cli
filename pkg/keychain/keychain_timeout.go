package keychain

import (
	"errors"
	"fmt"
	"time"
)

// keyringTimeout bounds a single OS keyring operation.
//
// Secret Service can block indefinitely when the D-Bus prompter cannot spawn —
// headless sessions, SSH without a desktop, containers with a socket but no
// agent — and 99designs/keyring offers no cancellation of its own. Without a
// bound, `astro` hangs with no output instead of failing.
const keyringTimeout = 3 * time.Second

// ErrTimeout is returned when the OS keyring does not answer within
// keyringTimeout.
var ErrTimeout = errors.New("timed out talking to the system keyring")

// timeoutStore bounds every call to the underlying store.
//
// A timed-out call leaks its goroutine until the process exits: the underlying
// library gives us no way to cancel an in-flight request, so the alternative is
// blocking forever. The CLI is short-lived, so the leak is bounded by the
// command.
//
// A timed-out SetCredentials is genuinely ambiguous — the write may still land
// afterwards. Callers must treat it as failed and not scrub any source copy of
// the credentials on the strength of it.
type timeoutStore struct {
	inner SecureStore
	d     time.Duration
}

func withTimeout(inner SecureStore, d time.Duration) SecureStore {
	return &timeoutStore{inner: inner, d: d}
}

func (t *timeoutStore) GetCredentials(domain string) (Credentials, error) {
	type result struct {
		creds Credentials
		err   error
	}
	ch := make(chan result, 1)
	go func() { c, err := t.inner.GetCredentials(domain); ch <- result{c, err} }()
	select {
	case r := <-ch:
		return r.creds, r.err
	case <-time.After(t.d):
		return Credentials{}, fmt.Errorf("%w after %s (reading credentials for %q)", ErrTimeout, t.d, domain)
	}
}

func (t *timeoutStore) SetCredentials(domain string, creds Credentials) error {
	ch := make(chan error, 1)
	go func() { ch <- t.inner.SetCredentials(domain, creds) }()
	select {
	case err := <-ch:
		return err
	case <-time.After(t.d):
		return fmt.Errorf("%w after %s (saving credentials for %q)", ErrTimeout, t.d, domain)
	}
}

func (t *timeoutStore) DeleteCredentials(domain string) error {
	ch := make(chan error, 1)
	go func() { ch <- t.inner.DeleteCredentials(domain) }()
	select {
	case err := <-ch:
		return err
	case <-time.After(t.d):
		return fmt.Errorf("%w after %s (deleting credentials for %q)", ErrTimeout, t.d, domain)
	}
}
