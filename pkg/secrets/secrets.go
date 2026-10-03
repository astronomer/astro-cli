// Package secrets stores sensitive values for local development. The
// mechanism: one master key in the OS keyring, values AES-256-GCM encrypted
// on disk in per-key files, with the cipher scoped per Store instance and the
// keyring service name a constructor parameter. crypto.go describes the
// envelope.
//
// The CLI and Astro Desktop share one vault: both open the store with
// DefaultService and DefaultDir, so a secret saved in either is readable in
// both. Call DefaultDir rather than joining "secrets" onto a home of your own —
// the location is the interop, and two callers deriving it separately is how one
// vault becomes two.
//
// # Threat model
//
// The vault protects values at rest. The master key never touches a regular
// file, so a copy of ~/.astro — a backup, a synced or accidentally shared
// dotfiles repository, a disk image, a file scraper, an AI agent reading files
// in the home directory, another account on the same machine — holds
// ciphertext and nothing that opens it.
//
// It does not protect against code running as the user, and is not meant to.
// The OS keyring scopes the master key to the user account rather than to
// either tool: on macOS, Windows and Linux alike, a process running as the
// user can ask the keyring for it. Such a process can equally read the
// environment of a running Airflow, which holds the decrypted values, or run
// `astro local env get`. This is the posture of a common developer tool, and
// the intended one; restricting the key to signed applications is not planned.
//
// Integrity holds against anyone who can write the vault directory but cannot
// get the key: a secret entry is served only from an envelope that
// authenticates under the master key and is bound to the entry's own key
// name. A planted plaintext value is refused (ErrUnencrypted), a ciphertext
// moved between entries fails to open (ErrTampered), and a value sealed under
// another master key says so (ErrWrongMasterKey). Two limits remain. A plain
// entry (SetPlain) is unauthenticated by design, since it must read without the
// keyring, so such a writer can plant one; ListMeta reports it as plain, never
// as a secret. And until a vault has been swept by Upgrade, its older v1
// values, which are not bound to their key, can still be moved between
// entries. The directory itself is checked on every use: a symlinked or
// foreign-owned one is refused (ErrVaultDirUnsafe), and one open to group or
// others is narrowed to the owner.
package secrets

import "errors"

// DefaultService is the OS keyring service name for the shared vault. Both
// the CLI and desktop use it; see Config.Service before changing anything.
const DefaultService = "astro"

// Meta identifies a stored value without exposing it. ListMeta is the only
// call allowed on surfaces visible to coding agents / LLMs (chat panes, MCP
// tools, AGENTS.md-driven flows): it reads the value files' keys and markers
// and is structurally incapable of returning a value, touching the keyring, or
// prompting — so an agent can see what exists but never what it is.
type Meta struct {
	Key string
	// Plain marks an entry stored unencrypted, by SetPlain: a value its owner
	// chose not to protect, which a consumer shows and passes on as plain
	// rather than as a secret. False for every entry Set wrote, and for every
	// file written before the marker existed. It is read from the file and not
	// authenticated, so it says how an entry is stored, not who stored it.
	Plain bool
}

// Store holds values: secret ones, encrypted under the master key, and
// through PlainSetter, plain ones beside them.
type Store interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	ListMeta() ([]Meta, error)
}

// PlainSetter stores a value unencrypted, marked plain, in the same layout as
// a secret one. Get returns it without the keyring, and ListMeta reports it
// with Meta.Plain set. Setting a key with Set or SetPlain replaces whatever
// the key held, secret or plain, so one key is always one entry.
//
// A separate interface rather than a Store method so a Store a caller wraps or
// fakes keeps compiling; the store NewKeyringStore returns implements it.
type PlainSetter interface {
	SetPlain(key, value string) error
}

// ErrPlainUnsupported is SetPlain's error for a Store that cannot hold a
// plain value.
var ErrPlainUnsupported = errors.New("this store cannot hold a plain value")

// SetPlain stores value under key in s unencrypted, marked plain: see
// PlainSetter. It fails with ErrPlainUnsupported when s does not implement it.
func SetPlain(s Store, key, value string) error {
	p, ok := s.(PlainSetter)
	if !ok {
		return ErrPlainUnsupported
	}
	return p.SetPlain(key, value)
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
