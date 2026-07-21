// Package secrets stores sensitive values for local development. The
// mechanism: one master key in the OS keyring, values AES-256-GCM encrypted
// on disk, with the AEAD scoped per Store instance and the keyring service
// name a constructor parameter.
//
// The CLI and Astro Desktop share one vault: both open the store with
// DefaultService and the shared value home (~/.astro/secrets), so a secret
// saved in either is readable in both. Desktop adopts the shared vault
// directly — it has no installed users, so there is nothing to migrate.
package secrets

import "errors"

// DefaultService is the OS keyring service name for the shared vault. Both
// the CLI and desktop use it; see Config.Service before changing anything.
const DefaultService = "astro"

// Meta identifies a stored secret without exposing its value. ListMeta is
// the only call allowed on surfaces visible to coding agents / LLMs (chat
// panes, MCP tools, AGENTS.md-driven flows): it reads cached ciphertext and
// is structurally incapable of returning a value, touching the keyring, or
// prompting — so an agent can see what exists but never what it is.
type Meta struct {
	Key string
}

// Store holds secret values.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	ListMeta() ([]Meta, error)
}

// ErrNotFound is returned by Get and Delete for a key that does not exist.
var ErrNotFound = errors.New("secret not found")

// Config configures a keyring-backed store.
type Config struct {
	// Service is the OS keyring service name holding the master key. Never
	// rename an existing service: the key, and every value encrypted under
	// it, is orphaned.
	Service string
	// Dir is where encrypted values live on disk.
	Dir string
}
