package secrets

import (
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// ErrKeyringUnavailable is the umbrella: the vault as a whole cannot be opened,
// so no value in it can be read. Every such error wraps it, which is what lets a
// caller tell this from a single corrupt value — see aead. There is no fallback
// yet (open decision in an earlier fix); the store refuses loudly instead.
//
// On its own it means the OS keyring is unreachable: headless Linux without a
// Secret Service, most CI, a locked or denied keychain. Retrying can help, and
// the remediation is about the machine.
var ErrKeyringUnavailable = errors.New("os keyring unavailable")

// ErrMasterKeyUnusable reports a keyring entry that exists but is not a usable
// key. It wraps ErrKeyringUnavailable so callers testing only the umbrella keep
// working, and is separate because the remediation is the opposite kind: nothing
// about the machine is wrong, and recovering means accepting that every value
// encrypted under the real key is unreadable. Saying "os keyring unavailable"
// alone would send someone to check a daemon that is running fine.
var ErrMasterKeyUnusable = fmt.Errorf("%w: master key is unusable", ErrKeyringUnavailable)

// ErrVaultOrphaned reports encrypted values with no master key: the keyring
// entry is gone (a keychain reset, a new login keychain, a recreated Linux
// keyring) while the value files remain.
//
// This exists because the alternative is silent and destructive. Minting a
// replacement key on demand is right for a first-ever use and wrong here: the
// new key decrypts nothing, so every existing value fails with an ordinary
// authentication error that is indistinguishable from one corrupt entry — a
// caller would report twenty broken values instead of one lost key — and the
// first subsequent write would leave two key generations in one directory with
// no way to tell them apart. Refusing keeps the ciphertext intact and the
// condition legible; recovery is deleting the directory, which is the user's
// call to make and not this package's.
var ErrVaultOrphaned = fmt.Errorf("%w: encrypted values exist but the master key is gone", ErrKeyringUnavailable)

const (
	keyringAccount = "master-key"
	keyBytes       = 32 // AES-256
	dirPerm        = 0o700
)

// keyringAPI seams the OS keyring so tests never touch the real one.
type keyringAPI interface {
	Get(service, account string) (string, error)
	Set(service, account, value string) error
}

type osKeyring struct{}

func (osKeyring) Get(service, account string) (string, error) {
	return keyring.Get(service, account)
}

func (osKeyring) Set(service, account, value string) error {
	return keyring.Set(service, account, value)
}

// NewKeyringStore opens the master-key + encrypted-file store: one AES-256
// key per Config.Service in the OS keyring, values AES-256-GCM encrypted in
// per-key files under Config.Dir. Construction does no I/O; the keyring is
// first touched by the first Get or Set that needs the key, so a store used
// only for ListMeta or Delete never prompts.
func NewKeyringStore(cfg Config) (Store, error) {
	return newKeyringStore(cfg, osKeyring{})
}

func newKeyringStore(cfg Config, kr keyringAPI) (*keyringStore, error) {
	if cfg.Service == "" {
		return nil, errors.New("Config.Service is empty")
	}
	if cfg.Dir == "" {
		return nil, errors.New("Config.Dir is empty")
	}
	return &keyringStore{service: cfg.Service, dir: cfg.Dir, kr: kr}, nil
}

type keyringStore struct {
	service string
	dir     string
	kr      keyringAPI

	// The AEAD is per instance, never a package global: two stores with
	// different services must hold different keys at the same time (the
	// in-memory test store beside a real one, or two vaults in one process).
	// Success is cached; failure is not, so a transient keyring error does
	// not wedge a long-lived store.
	mu  sync.Mutex
	gcm cipher.AEAD
}

// aead returns the store's cipher, initialized from the OS keyring on first
// use.
//
// Every failure path wraps ErrKeyringUnavailable, because from a caller's
// perspective they are one condition: the vault cannot be opened, so no value
// in it can be read. That matters more than it looks. A consumer reading many
// keys has to tell "this one value is corrupt" (skip it, keep serving the rest)
// from "nothing will work" (fail the read) — and only success is cached here, so
// a failure that reads as per-entry gets retried for every entry, re-execing the
// OS keyring N times and, on macOS, able to raise N keychain dialogs. A
// malformed master key used to arrive as a bare "decode master key" error and do
// exactly that.
//
// The one whole-vault failure this cannot express as an aead error is a lost key
// with values still on disk, because a replacement key builds a perfectly valid
// cipher that simply decrypts nothing. masterKey refuses that case up front
// instead — see ErrVaultOrphaned.
func (s *keyringStore) aead() (cipher.AEAD, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.gcm != nil {
		return s.gcm, nil
	}
	key, err := s.masterKey()
	if err != nil {
		return nil, err
	}
	// Unreachable with a key masterKey produced (it guarantees keyBytes, and AES
	// accepts 32 bytes), and wrapped anyway so the invariant above holds for
	// whatever a future key source does.
	gcm, err := newAEAD(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeyringUnavailable, err)
	}
	s.gcm = gcm
	return gcm, nil
}

func (s *keyringStore) masterKey() ([]byte, error) {
	stored, err := s.kr.Get(s.service, keyringAccount)
	if err == nil {
		key, err := base64.StdEncoding.DecodeString(stored)
		if err != nil {
			return nil, fmt.Errorf("%w: not base64: %w", ErrMasterKeyUnusable, err)
		}
		if len(key) != keyBytes {
			return nil, fmt.Errorf("%w: length %d, want %d", ErrMasterKeyUnusable, len(key), keyBytes)
		}
		return key, nil
	}
	// Any failure other than "no entry yet" means we cannot reach the keyring at
	// all. %w rather than %v so a caller can still reach the platform's own
	// reason: keyring.ErrUnsupportedPlatform is permanent and means stop asking,
	// while a dbus error is transient and worth retrying once the session is up,
	// and only the wrapped chain can tell them apart.
	if !errors.Is(err, keyring.ErrNotFound) {
		return nil, fmt.Errorf("%w: read master key: %w", ErrKeyringUnavailable, err)
	}
	// No key. Minting one is right for a vault that has never been used and
	// destructive for a vault that has: see ErrVaultOrphaned.
	orphaned, err := s.hasValues()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeyringUnavailable, err)
	}
	if orphaned {
		return nil, ErrVaultOrphaned
	}
	// Two processes racing the very first key creation can each generate a
	// key and the loser's values are orphaned. The OS keyring has no
	// compare-and-swap, the window exists once per vault ever, and desktop
	// carries the same behavior — accepted.
	key := make([]byte, keyBytes)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("%w: generate master key: %w", ErrKeyringUnavailable, err)
	}
	if err := s.kr.Set(s.service, keyringAccount, base64.StdEncoding.EncodeToString(key)); err != nil {
		return nil, fmt.Errorf("%w: persist master key: %w", ErrKeyringUnavailable, err)
	}
	return key, nil
}

// hasValues reports whether the vault directory holds any stored value, which is
// what separates "never used" from "the key is gone".
func (s *keyringStore) hasValues() (bool, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("list secrets: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), valueExt) {
			return true, nil
		}
	}
	return false, nil
}

// valueFile is the on-disk JSON for one secret. The key lives inside the
// file, not in the filename: keys may be longer than a filename allows and
// may carry characters (colons, slashes) that filesystems reject, so the
// filename is a hash of the key and ListMeta reads keys back from the files.
type valueFile struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

const valueExt = ".json"

func (s *keyringStore) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(s.dir, hex.EncodeToString(sum[:])+valueExt)
}

func readValueFile(path string) (valueFile, error) {
	var vf valueFile
	raw, err := os.ReadFile(path)
	if err != nil {
		return vf, err
	}
	if err := json.Unmarshal(raw, &vf); err != nil {
		return vf, fmt.Errorf("parse secret file: %w", err)
	}
	return vf, nil
}

func (s *keyringStore) Get(key string) (string, error) {
	// Read the file before touching the keyring so a missing key reports
	// ErrNotFound without ever prompting.
	vf, err := readValueFile(s.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("read secret: %w", err)
	}
	gcm, err := s.aead()
	if err != nil {
		return "", err
	}
	return decrypt(gcm, vf.Value)
}

func (s *keyringStore) Set(key, value string) error {
	gcm, err := s.aead()
	if err != nil {
		return err
	}
	enc, err := encrypt(gcm, value)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(valueFile{Key: key, Value: enc})
	if err != nil {
		return fmt.Errorf("encode secret file: %w", err)
	}
	if err := os.MkdirAll(s.dir, dirPerm); err != nil {
		return fmt.Errorf("create secrets dir: %w", err)
	}
	// Write-then-rename: the CLI and desktop share this directory, so a
	// concurrent reader must see the old value or the new one, never a torn
	// file. Per-value files also keep writers to different keys from ever
	// contending, which a single flocked file would not.
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup of a temp file we are about to rename away
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmp.Name(), s.path(key)); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	return nil
}

func (s *keyringStore) Delete(key string) error {
	err := os.Remove(s.path(key))
	if errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

// ListMeta reads the cached ciphertext files and nothing else. It has no
// path to the AEAD or the keyring, so it can never return a value, prompt,
// or fail on a headless machine.
func (s *keyringStore) ListMeta() ([]Meta, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []Meta{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	metas := make([]Meta, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), valueExt) {
			continue // in-flight temp files and strays
		}
		vf, err := readValueFile(filepath.Join(s.dir, e.Name()))
		if errors.Is(err, os.ErrNotExist) {
			continue // deleted between ReadDir and here
		}
		if err != nil {
			return nil, fmt.Errorf("list secrets: %w", err)
		}
		metas = append(metas, Meta{Key: vf.Key})
	}
	return metas, nil
}
