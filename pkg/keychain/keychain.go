package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/99designs/keyring"
)

const serviceName = "astro-cli"

// ErrNotFound is returned when no credentials exist for the given domain.
var ErrNotFound = errors.New("credentials not found")

// ErrInsecureFallbackRefused is returned by New when no OS-native secure store
// is available and the user has set no_insecure_fallback, meaning they would
// rather fail than have credentials written to disk in the clear.
var ErrInsecureFallbackRefused = errors.New("no secure credential store available and the plaintext fallback is disabled")

var (
	credentialsDir   string
	credentialsDirMu sync.RWMutex
)

// SetCredentialsDir sets the directory the plaintext fallback store writes to.
// Must be called before New on any platform that can fall back to a file.
//
// Injected rather than resolved here because the astro home directory honors
// ASTRO_HOME and the repo's own home-dir lookup, both of which live in package
// config — which imports this package, so this one cannot import it back.
// Resolving it independently would drift: credentials would land in ~/.astro
// while config.yaml went to $ASTRO_HOME/.astro, splitting one login across two
// directories. Same injection pattern as pkg/proxy.SetRoutesDir.
func SetCredentialsDir(dir string) {
	credentialsDirMu.Lock()
	defer credentialsDirMu.Unlock()
	credentialsDir = dir
}

// CredentialsDir returns the configured credentials directory.
func CredentialsDir() string {
	credentialsDirMu.RLock()
	defer credentialsDirMu.RUnlock()
	return credentialsDir
}

// SecureStore persists and retrieves authentication credentials
// using the OS-native secure store.
type SecureStore interface {
	GetCredentials(domain string) (Credentials, error)
	SetCredentials(domain string, creds Credentials) error
	DeleteCredentials(domain string) error
}

// Credentials holds all authentication credentials for a single context.
type Credentials struct {
	Token        string    `json:"token"`
	RefreshToken string    `json:"refreshtoken"`
	UserEmail    string    `json:"user_email"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// keyringStore is the SecureStore implementation backed by a
// 99designs/keyring.Keyring — macOS Keychain, or Secret Service / KWallet on
// Linux.
type keyringStore struct {
	ring keyring.Keyring
}

func (s *keyringStore) GetCredentials(domain string) (Credentials, error) {
	item, err := s.ring.Get(domain)
	if errors.Is(err, keyring.ErrKeyNotFound) {
		return Credentials{}, ErrNotFound
	}
	if err != nil {
		return Credentials{}, fmt.Errorf("reading credentials: %w", err)
	}
	var creds Credentials
	if err := json.Unmarshal(item.Data, &creds); err != nil {
		return Credentials{}, fmt.Errorf("decoding credentials: %w", err)
	}
	return creds, nil
}

func (s *keyringStore) SetCredentials(domain string, creds Credentials) error {
	data, err := json.Marshal(creds)
	if err != nil {
		return fmt.Errorf("encoding credentials: %w", err)
	}
	if err := s.ring.Set(keyring.Item{Key: domain, Label: "Astro CLI (" + domain + ")", Data: data}); err != nil {
		return fmt.Errorf("writing credentials: %w", err)
	}
	return nil
}

func (s *keyringStore) DeleteCredentials(domain string) error {
	err := s.ring.Remove(domain)
	if err == nil || errors.Is(err, keyring.ErrKeyNotFound) {
		return nil
	}
	return fmt.Errorf("deleting credentials: %w", err)
}

// NewTestStore returns an in-memory SecureStore for use in unit tests.
// It is backed by keyring.NewArrayKeyring which ships with 99designs/keyring.
func NewTestStore() SecureStore {
	return &keyringStore{ring: keyring.NewArrayKeyring(nil)}
}
