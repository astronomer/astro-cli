package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
)

// upgradeLock serializes Upgrade sweeps between processes. Not ".json", so
// ListMeta and hasValues skip it.
const upgradeLock = "upgrade.lock"

// Upgrader rewrites values stored in an older envelope in the current one: see
// Upgrade. A separate interface, like PlainSetter, so a Store a caller wraps or
// fakes keeps compiling; the store NewKeyringStore returns implements it.
type Upgrader interface {
	Upgrade() (UpgradeReport, error)
}

// UpgradeReport is what one Upgrade did.
type UpgradeReport struct {
	// Upgraded counts the entries rewritten in the current envelope.
	Upgraded int
	// Skipped are the entries that needed upgrading and were left as they
	// were, each with the reason. A skipped entry is never deleted or
	// rewritten: a v1 value that could not be opened stays on disk exactly as
	// found, for a later sweep under the right key or for the user to remove.
	Skipped []UpgradeSkip
}

// UpgradeSkip is one entry Upgrade left alone. Key is the vault key its file
// names, which identifies the entry and never carries its value.
type UpgradeSkip struct {
	Key string
	Err error
}

// Upgrade runs s's upgrade sweep when it has one, and otherwise does nothing:
// a store that does not implement Upgrader has no older envelope to migrate.
func Upgrade(s Store) (UpgradeReport, error) {
	u, ok := s.(Upgrader)
	if !ok {
		return UpgradeReport{}, nil
	}
	return u.Upgrade()
}

// Upgrade rewrites every secret entry still in the older form as a v2
// envelope: enc:v1: values, which are not bound to their key and carry no key
// id, and the empty string, which older builds stored unencrypted for an empty
// secret. Get reads both until then; after a sweep neither is left.
//
// It is an explicit sweep rather than a rewrite inside Get, because a read
// that writes races an ordinary Set from the other tool: Get reads the old
// value, the other tool stores a new one, and Get's rewrite then puts the old
// value back. Here every rewrite is guarded instead: the file is read again
// just before publishing and left alone if it changed since it was decrypted.
// The guard narrows that race to the few calls between its read and the rename
// rather than closing it, since Set takes no lock; a Set landing in exactly
// that window is overwritten by the value it replaced.
//
// The keyring is touched only when there is something to upgrade, so calling
// this on every start costs a directory listing once a vault is current and
// raises no prompt. Holding upgradeLock keeps the CLI and the desktop from
// sweeping at once.
//
// Upgrading trusts what it upgrades: a v1 value moved into another entry's
// file, or an empty value planted in one, before the sweep is rewritten as
// that entry's value. That is the price of reading the older form at all, and
// it ends once a vault has been swept. An entry whose file is not stored under
// its own key's name is never upgraded.
//
// It fails as a whole only when the vault cannot be opened (an unsafe
// directory, the lock, the master key). Any per-entry problem is a Skip.
func (s *keyringStore) Upgrade() (UpgradeReport, error) {
	var report UpgradeReport
	exists, err := prepareDir(s.dir, false)
	if err != nil || !exists {
		return report, err
	}
	pending, err := s.pendingUpgrades()
	if err != nil || len(pending) == 0 {
		return report, err
	}

	unlock, err := fsatomic.Lock(filepath.Join(s.dir, upgradeLock))
	if err != nil {
		return report, fmt.Errorf("lock the vault for upgrade: %w", err)
	}
	defer unlock()
	c, err := s.aead()
	if err != nil {
		return report, err
	}
	for _, path := range pending {
		key, upgraded, err := s.upgradeFile(c, path)
		switch {
		case err != nil:
			report.Skipped = append(report.Skipped, UpgradeSkip{Key: key, Err: err})
		case upgraded:
			report.Upgraded++
		}
	}
	return report, nil
}

// needsUpgrade reports whether a value file holds a secret in the older form.
// It reads the value's shape only, never decrypts, so it needs no key.
func needsUpgrade(vf valueFile) bool {
	return !vf.Plain && (vf.Value == "" || strings.HasPrefix(vf.Value, encPrefixV1))
}

// pendingUpgrades lists the value files that need upgrading, without the key.
func (s *keyringStore) pendingUpgrades() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	var pending []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), valueExt) {
			continue
		}
		path := filepath.Join(s.dir, e.Name())
		vf, err := readValueFile(path)
		if err != nil {
			continue // gone, or not a value file this sweep could rewrite
		}
		if needsUpgrade(vf) {
			pending = append(pending, path)
		}
	}
	return pending, nil
}

// upgradeFile rewrites one value file as v2 and reports whether it did. key is
// the key the file names, for the report, even when it fails.
func (s *keyringStore) upgradeFile(c *vaultCipher, path string) (key string, upgraded bool, err error) {
	raw, err := fsatomic.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil // deleted since it was listed
	}
	if err != nil {
		return "", false, fmt.Errorf("read secret: %w", err)
	}
	var vf valueFile
	if err := json.Unmarshal(raw, &vf); err != nil {
		return "", false, fmt.Errorf("parse secret file: %w", err)
	}
	if !needsUpgrade(vf) {
		return vf.Key, false, nil // rewritten since it was listed
	}
	if s.path(vf.Key) != path {
		return vf.Key, false, fmt.Errorf("%w: %s is not the file for the key it names", ErrTampered, filepath.Base(path))
	}
	plain := ""
	if vf.Value != "" {
		if plain, _, err = c.open(vf.Key, vf.Value); err != nil {
			return vf.Key, false, err
		}
	}
	enc, err := c.seal(vf.Key, plain)
	if err != nil {
		return vf.Key, false, err
	}
	out, err := json.Marshal(valueFile{Key: vf.Key, Value: enc})
	if err != nil {
		return vf.Key, false, fmt.Errorf("encode secret file: %w", err)
	}
	// The guard against a concurrent Set: what is on disk now must still be
	// what was decrypted, or the newer value is left in place.
	now, err := fsatomic.ReadFile(path)
	if err != nil || !bytes.Equal(now, raw) {
		return vf.Key, false, nil
	}
	if err := fsatomic.WriteFile(path, out, valuePerm); err != nil {
		return vf.Key, false, err
	}
	return vf.Key, true, nil
}
