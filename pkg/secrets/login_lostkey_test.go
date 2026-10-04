package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// loseMasterKey removes the master key from kr, as a keychain reset does, and
// returns the logins of a new process, which has no cipher cached.
func loseMasterKey(t *testing.T, kr *fakeKeyring, dir string) *Logins {
	t.Helper()
	kr.mu.Lock()
	delete(kr.entries, DefaultService+"\x00"+keyringAccount)
	kr.mu.Unlock()
	l, err := newLogins(dir, kr)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func valueFiles(t *testing.T, dir string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "*"+valueExt))
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func masterKeyStored(kr *fakeKeyring) bool {
	kr.mu.Lock()
	defer kr.mu.Unlock()
	_, ok := kr.entries[DefaultService+"\x00"+keyringAccount]
	return ok
}

// Logins left behind by a lost master key do not keep a new key from being
// made: they read as signed out, are removed, and the next login is stored in
// the vault. A plain value and the marker keeping another context in the
// config stay.
func TestALostMasterKeyWithOnlyLoginsMintsANewKey(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	for _, ctx := range []string{"astronomer_io", "other_io"} {
		if fields := l.Save(cfg, ctx, testLogin); fields != configFields {
			t.Fatalf("setup: %s was not saved in the vault", ctx)
		}
	}
	kept := loginKey(cfg, "kept_io")
	l.keepInConfig(kept)
	if err := l.store.SetPlain("env:global:PLAIN", "value"); err != nil {
		t.Fatal(err)
	}

	l = loseMasterKey(t, kr, dir)
	got, err := l.Read(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatalf("Read after the key was lost: %v", err)
	}
	sameLogin(t, "login read under a new key", got, Login{})
	if !masterKeyStored(kr) {
		t.Fatal("no new master key was made")
	}
	if n := len(valueFiles(t, dir)); n != 1 {
		t.Errorf("entries left after the new key = %d, want only the plain value", n)
	}
	if v, err := l.store.Get("env:global:PLAIN"); err != nil || v != "value" {
		t.Errorf("the plain value did not survive the new key: %v", err)
	}
	if !l.keptInConfig(kept) {
		t.Error("the marker keeping a context in the config was removed")
	}

	if fields := l.Save(cfg, "astronomer_io", testLogin2); fields != configFields {
		t.Fatal("a login after the new key was not stored in the vault")
	}
	got, err = l.Read(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sameLogin(t, "login saved under the new key", got, testLogin2)
}

// A login in the config moves into the vault after a lost key, in a process
// whose first vault operation is that move.
func TestALostMasterKeyLetsAPlaintextLoginMoveIn(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "other_io", testLogin)

	l = loseMasterKey(t, kr, dir)
	got, rewrite, err := l.Resolve(cfg, "astronomer_io", testLogin2)
	if err != nil || rewrite == nil || *rewrite != configFields {
		t.Fatalf("Resolve: moved = %v, err = %v", rewrite != nil, err)
	}
	sameLogin(t, "resolved login", got, testLogin2)
	got, err = l.Read(cfg, "other_io", configFields)
	if err != nil {
		t.Fatalf("Read of a login left by the lost key: %v", err)
	}
	sameLogin(t, "login left by the lost key", got, Login{})
	if n := len(valueFiles(t, dir)); n != 1 {
		t.Errorf("vault entries = %d, want only the moved login", n)
	}
}

// Other encrypted values still refuse a new key, logins or not, and nothing
// is removed.
func TestALostMasterKeyWithOtherValuesIsStillRefused(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	if err := l.store.Set("env:global:WAREHOUSE", "value"); err != nil {
		t.Fatal(err)
	}

	l = loseMasterKey(t, kr, dir)
	if _, err := l.Read(cfg, "astronomer_io", configFields); !errors.Is(err, ErrVaultOrphaned) {
		t.Errorf("Read = %v, want ErrVaultOrphaned", err)
	}
	if masterKeyStored(kr) {
		t.Error("a new master key was made over encrypted values")
	}
	if n := len(valueFiles(t, dir)); n != 2 {
		t.Errorf("vault entries = %d, want both left alone", n)
	}
}

// A value that does not decrypt and is still there is reported unreadable,
// not missing.
func TestAnUndecryptableValueIsNotReportedMissing(t *testing.T) {
	kr := newFakeKeyring()
	s := testStore(t, kr, DefaultService, t.TempDir())
	if err := s.Set("env:global:A", "value"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.path("env:global:B"), []byte(`{"key":"env:global:B","value":"not-ciphertext"}`), valuePerm); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get("env:global:B"); err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("Get of a corrupt value = %v, want an error other than ErrNotFound", err)
	}
}

// racingKeyring answers its first read as if no key existed, and stores key
// right after, as another process making the first key at the same moment
// does.
type racingKeyring struct {
	*fakeKeyring
	key   string
	raced bool
}

func (r *racingKeyring) Get(service, account string) (string, error) {
	if !r.raced {
		r.raced = true
		v, err := r.fakeKeyring.Get(service, account+"-absent")
		_ = r.fakeKeyring.Set(service, account, r.key)
		return v, err
	}
	return r.fakeKeyring.Get(service, account)
}

// Another process made a key and stored a login under it after this one
// found no key: the login is kept and read with that key, not removed.
func TestALoginStoredUnderAKeyJustMadeIsKept(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	kr.mu.Lock()
	key := kr.entries[DefaultService+"\x00"+keyringAccount]
	delete(kr.entries, DefaultService+"\x00"+keyringAccount)
	kr.mu.Unlock()

	l, err := newLogins(dir, &racingKeyring{fakeKeyring: kr, key: key})
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Read(cfg, "astronomer_io", configFields)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	sameLogin(t, "login stored under the other process's key", got, testLogin)
}

// The keyring fails on the second look rather than answering that there is
// no key: nothing is removed and no key is made.
func TestALostMasterKeyRemovesNothingWhenTheKeyringFails(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	loseMasterKey(t, kr, dir)
	l, err := newLogins(dir, &failingSecondGet{fakeKeyring: kr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.Read(cfg, "astronomer_io", configFields); !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("Read = %v, want ErrKeyringUnavailable", err)
	}
	if masterKeyStored(kr) {
		t.Error("a key was made although the keyring failed")
	}
	if n := len(valueFiles(t, dir)); n != 1 {
		t.Errorf("vault entries = %d, want the login left alone", n)
	}
}

// failingSecondGet answers its first read normally and fails every later one.
type failingSecondGet struct {
	*fakeKeyring
	gets int
}

func (f *failingSecondGet) Get(service, account string) (string, error) {
	f.gets++
	if f.gets > 1 {
		return "", errors.New("the keyring stopped answering")
	}
	return f.fakeKeyring.Get(service, account)
}

// A login entry in a file named for another key is not taken for a login:
// it refuses a new key like any other value, and is not removed.
func TestALoginInAFileNamedForAnotherKeyStillRefuses(t *testing.T) {
	kr := newFakeKeyring()
	l, dir := testLogins(t, kr)
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	l.Save(cfg, "astronomer_io", testLogin)
	src := l.store.path(loginKey(cfg, "astronomer_io"))
	if err := os.Rename(src, l.store.path("env:global:OTHER")); err != nil {
		t.Fatal(err)
	}
	l = loseMasterKey(t, kr, dir)
	if _, err := l.Read(cfg, "other_io", configFields); err != nil {
		t.Fatalf("Read of a context with no entry: %v", err)
	}
	if err := l.store.Set("env:global:NEW", "v"); !errors.Is(err, ErrVaultOrphaned) {
		t.Errorf("Set = %v, want ErrVaultOrphaned", err)
	}
	if n := len(valueFiles(t, dir)); n != 1 {
		t.Errorf("vault entries = %d, want the misnamed file left alone", n)
	}
}
