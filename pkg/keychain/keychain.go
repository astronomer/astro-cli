package keychain

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/99designs/keyring"
)

const serviceName = "astro-cli"

// ErrNotFound is returned when no credentials exist for the given domain.
var ErrNotFound = errors.New("credentials not found")

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

// keyringStore is the shared SecureStore implementation for macOS and Linux
// Secret Service, backed by a 99designs/keyring.Keyring.
//
// On Windows, see keychain_windows.go for the per-field implementation.
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
