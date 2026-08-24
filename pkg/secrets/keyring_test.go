package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zalando/go-keyring"
)

// fakeKeyring is an in-memory stand-in so tests never touch the OS keyring.
type fakeKeyring struct {
	mu      sync.Mutex
	entries map[string]string
	sets    int
	gets    int
	err     error // when non-nil, every call fails with it
}

func newFakeKeyring() *fakeKeyring {
	return &fakeKeyring{entries: map[string]string{}}
}

func (f *fakeKeyring) Get(service, account string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets++
	if f.err != nil {
		return "", f.err
	}
	v, ok := f.entries[service+"\x00"+account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}

func (f *fakeKeyring) Set(service, account, value string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sets++
	if f.err != nil {
		return f.err
	}
	f.entries[service+"\x00"+account] = value
	return nil
}

func testStore(t *testing.T, kr keyringAPI, service, dir string) *keyringStore {
	t.Helper()
	s, err := newKeyringStore(Config{Service: service, Dir: dir}, kr)
	if err != nil {
		t.Fatalf("newKeyringStore: %v", err)
	}
	return s
}

func TestKeyringStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro-test", dir)

	keys := map[string]string{
		"plain":                          "value1",
		"env:global:SNOWFLAKE_PASSWORD":  "hunter2",
		"conn:/Users/x/proj worktree:db": "postgres://u:p@h/db",
		strings.Repeat("k", 300):         "long-key value",
		"unicode:ключ":                   "значение",
		"empty":                          "",
	}
	for k, v := range keys {
		if err := s.Set(k, v); err != nil {
			t.Fatalf("Set(%q): %v", k, err)
		}
	}
	for k, want := range keys {
		got, err := s.Get(k)
		if err != nil || got != want {
			t.Fatalf("Get(%q) = %q, %v; want %q, nil", k, got, err, want)
		}
	}

	// A second instance over the same dir and keyring reads the same vault.
	s2 := testStore(t, kr, "astro-test", dir)
	for k, want := range keys {
		got, err := s2.Get(k)
		if err != nil || got != want {
			t.Fatalf("second store Get(%q) = %q, %v; want %q, nil", k, got, err, want)
		}
	}

	// Overwrite keeps the latest value.
	if err := s.Set("plain", "value2"); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if got, _ := s2.Get("plain"); got != "value2" {
		t.Fatalf("Get after overwrite = %q, want %q", got, "value2")
	}
}

func TestKeyringStoreNotFound(t *testing.T) {
	s := testStore(t, newFakeKeyring(), "astro-test", t.TempDir())

	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound", err)
	}
	if err := s.Delete("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) = %v, want ErrNotFound", err)
	}

	if err := s.Set("a", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := s.Delete("a"); err != nil {
		t.Fatalf("Delete(a): %v", err)
	}
	if _, err := s.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(a) after delete = %v, want ErrNotFound", err)
	}
}

func TestGetMissingNeverTouchesKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("no dbus")
	s := testStore(t, kr, "astro-test", t.TempDir())
	if _, err := s.Get("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(missing) = %v, want ErrNotFound even when keyring is down", err)
	}
}

func TestListMetaNeverDecryptsOrTouchesKeyring(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro-test", dir)
	if err := s.Set("env:global:PASSWORD", "hunter2"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The value must not sit in plaintext on disk.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		if strings.Contains(string(raw), "hunter2") {
			t.Fatalf("plaintext on disk in %s: %s", e.Name(), raw)
		}
	}

	// A fresh store whose keyring is completely down can still list.
	deadKr := newFakeKeyring()
	deadKr.err = errors.New("no dbus")
	s2 := testStore(t, deadKr, "astro-test", dir)
	metas, err := s2.ListMeta()
	if err != nil {
		t.Fatalf("ListMeta with dead keyring: %v", err)
	}
	if len(metas) != 1 || metas[0].Key != "env:global:PASSWORD" {
		t.Fatalf("ListMeta = %v, want the one key", metas)
	}
	if deadKr.gets != 0 || deadKr.sets != 0 {
		t.Fatalf("ListMeta touched the keyring: %d gets, %d sets", deadKr.gets, deadKr.sets)
	}

	// Delete works without the keyring too.
	if err := s2.Delete("env:global:PASSWORD"); err != nil {
		t.Fatalf("Delete with dead keyring: %v", err)
	}
}

func TestListMetaEmptyAndMissingDir(t *testing.T) {
	s := testStore(t, newFakeKeyring(), "astro-test", filepath.Join(t.TempDir(), "never-created"))
	metas, err := s.ListMeta()
	if err != nil {
		t.Fatalf("ListMeta on missing dir: %v", err)
	}
	if len(metas) != 0 {
		t.Fatalf("ListMeta on missing dir = %v, want empty", metas)
	}
}

func TestKeyringUnavailable(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("The name org.freedesktop.secrets was not provided by any .service files")
	s := testStore(t, kr, "astro-test", t.TempDir())

	if err := s.Set("k", "v"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("Set with dead keyring = %v, want ErrKeyringUnavailable", err)
	}

	// A store whose value exists on disk but whose keyring is down must
	// surface the same typed error from Get.
	dir := t.TempDir()
	okKr := newFakeKeyring()
	writer := testStore(t, okKr, "astro-test", dir)
	if err := writer.Set("k", "v"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	deadKr := newFakeKeyring()
	deadKr.err = errors.New("no dbus")
	reader := testStore(t, deadKr, "astro-test", dir)
	if _, err := reader.Get("k"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("Get with dead keyring = %v, want ErrKeyringUnavailable", err)
	}
}

func TestKeyringRecoversAfterTransientFailure(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("no dbus")
	s := testStore(t, kr, "astro-test", t.TempDir())

	if err := s.Set("k", "v"); !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("Set with dead keyring = %v, want ErrKeyringUnavailable", err)
	}

	// The failure must not be cached: once the keyring is back, the same
	// store works.
	kr.mu.Lock()
	kr.err = nil
	kr.mu.Unlock()
	if err := s.Set("k", "v"); err != nil {
		t.Fatalf("Set after keyring recovered: %v", err)
	}
	if got, err := s.Get("k"); err != nil || got != "v" {
		t.Fatalf("Get after recovery = %q, %v; want v", got, err)
	}
}

func TestTwoStoresCoexist(t *testing.T) {
	// Different services get different master keys, held at the same time in
	// one process — the point of the instance-scoped AEAD.
	dirA, dirB := t.TempDir(), t.TempDir()
	kr := newFakeKeyring()
	a := testStore(t, kr, "service-a", dirA)
	b := testStore(t, kr, "service-b", dirB)

	if err := a.Set("k", "from-a"); err != nil {
		t.Fatalf("a.Set: %v", err)
	}
	if err := b.Set("k", "from-b"); err != nil {
		t.Fatalf("b.Set: %v", err)
	}
	// Interleave to prove neither store clobbered the other's key.
	if got, err := a.Get("k"); err != nil || got != "from-a" {
		t.Fatalf("a.Get = %q, %v; want from-a", got, err)
	}
	if got, err := b.Get("k"); err != nil || got != "from-b" {
		t.Fatalf("b.Get = %q, %v; want from-b", got, err)
	}

	// A store under the wrong service cannot decrypt the other's file.
	wrong := testStore(t, kr, "service-b", dirA)
	if _, err := wrong.Get("k"); err == nil {
		t.Fatal("decrypting service-a's value with service-b's key should fail")
	}
}

func TestMasterKeyCreatedOnceAndReused(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()

	s1 := testStore(t, kr, "astro-test", dir)
	if err := s1.Set("a", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if kr.sets != 1 {
		t.Fatalf("first store wrote master key %d times, want 1", kr.sets)
	}

	s2 := testStore(t, kr, "astro-test", dir)
	if err := s2.Set("b", "2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if kr.sets != 1 {
		t.Fatalf("second store re-wrote master key: %d sets, want 1", kr.sets)
	}
	if got, err := s2.Get("a"); err != nil || got != "1" {
		t.Fatalf("s2.Get(a) = %q, %v; want 1", got, err)
	}
}

func TestConcurrentWriters(t *testing.T) {
	// Two store instances over one vault, like the CLI and desktop running
	// at once.
	dir := t.TempDir()
	kr := newFakeKeyring()
	a := testStore(t, kr, "astro-test", dir)
	b := testStore(t, kr, "astro-test", dir)

	// Seed the vault so the master key exists before both stores race;
	// concurrent first-ever creation is the documented accepted race.
	if err := a.Set("seed", "s"); err != nil {
		t.Fatalf("seed Set: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if err := a.Set(fmt.Sprintf("a-%d", i), fmt.Sprintf("va-%d", i)); err != nil {
				t.Errorf("a.Set: %v", err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if err := b.Set(fmt.Sprintf("b-%d", i), fmt.Sprintf("vb-%d", i)); err != nil {
				t.Errorf("b.Set: %v", err)
			}
		}(i)
	}
	// Both hammer one shared key at the same time.
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if err := a.Set("shared", fmt.Sprintf("sa-%d", i)); err != nil {
				t.Errorf("a.Set(shared): %v", err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if err := b.Set("shared", fmt.Sprintf("sb-%d", i)); err != nil {
				t.Errorf("b.Set(shared): %v", err)
			}
		}(i)
	}
	wg.Wait()

	for i := 0; i < n; i++ {
		if got, err := b.Get(fmt.Sprintf("a-%d", i)); err != nil || got != fmt.Sprintf("va-%d", i) {
			t.Fatalf("cross-read a-%d = %q, %v", i, got, err)
		}
		if got, err := a.Get(fmt.Sprintf("b-%d", i)); err != nil || got != fmt.Sprintf("vb-%d", i) {
			t.Fatalf("cross-read b-%d = %q, %v", i, got, err)
		}
	}
	// The shared key holds exactly one of the written values, intact.
	got, err := a.Get("shared")
	if err != nil {
		t.Fatalf("Get(shared): %v", err)
	}
	if !strings.HasPrefix(got, "sa-") && !strings.HasPrefix(got, "sb-") {
		t.Fatalf("Get(shared) = %q, not one of the written values", got)
	}

	metas, err := a.ListMeta()
	if err != nil {
		t.Fatalf("ListMeta: %v", err)
	}
	if len(metas) != 2*n+2 {
		t.Fatalf("ListMeta returned %d entries, want %d", len(metas), 2*n+2)
	}
}

func TestConfigValidation(t *testing.T) {
	if _, err := NewKeyringStore(Config{Dir: t.TempDir()}); err == nil {
		t.Fatal("empty Service should fail")
	}
	if _, err := NewKeyringStore(Config{Service: "astro"}); err == nil {
		t.Fatal("empty Dir should fail")
	}
}

// A master key that is present but unusable means the same thing to a caller as
// a keyring that cannot be reached: nothing in this vault can be read. It has to
// say so with the same sentinel.
//
// The distinction is load-bearing for a consumer reading many keys, which must
// tell "this one value is corrupt" (skip it, serve the rest) from "nothing will
// work" (fail the read). Only success is cached in aead(), so a whole-vault
// failure that reads as per-entry is retried for every entry — re-execing the OS
// keyring N times and, on macOS, able to raise N keychain dialogs.
func TestUnusableMasterKeyReportsTheVaultUnavailable(t *testing.T) {
	const key = "conn:global:warehouse"
	for name, corrupt := range map[string]string{
		"not base64":   "!!!not-base64!!!",
		"wrong length": base64.StdEncoding.EncodeToString([]byte("too short")),
	} {
		t.Run(name, func(t *testing.T) {
			// A value has to exist first: Get reads the file before the keyring,
			// so a missing key reports ErrNotFound without ever prompting, and
			// would never reach the master key at all.
			dir := t.TempDir()
			kr := newFakeKeyring()
			if err := testStore(t, kr, "astro", dir).Set(key, "topsecret"); err != nil {
				t.Fatalf("seed value: %v", err)
			}
			if err := kr.Set("astro", keyringAccount, corrupt); err != nil {
				t.Fatalf("corrupt the key: %v", err)
			}

			// A fresh store, because the first one cached its cipher.
			_, err := testStore(t, kr, "astro", dir).Get(key)
			if !errors.Is(err, ErrKeyringUnavailable) {
				t.Errorf("err = %v, want it to wrap ErrKeyringUnavailable", err)
			}
		})
	}
}

// Values on disk with no master key must be refused, not re-keyed.
//
// The keyring has no compare-and-swap and no history, so a missing entry is
// indistinguishable from a never-used vault by looking at the keyring alone —
// the value files are the only evidence. Minting a replacement builds a
// perfectly valid cipher that decrypts nothing, so every existing value would
// fail with an ordinary authentication error: a caller would report twenty
// corrupt entries rather than one lost key, and the first write after that would
// leave two key generations in one directory with nothing to tell them apart.
func TestLostMasterKeyWithValuesPresentIsRefused(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	if err := testStore(t, kr, "astro", dir).Set("conn:global:warehouse", "topsecret"); err != nil {
		t.Fatalf("seed value: %v", err)
	}

	// The keychain entry disappears; the files do not.
	kr.mu.Lock()
	delete(kr.entries, "astro\x00"+keyringAccount)
	kr.mu.Unlock()

	_, err := testStore(t, kr, "astro", dir).Get("conn:global:warehouse")
	if !errors.Is(err, ErrVaultOrphaned) {
		t.Errorf("err = %v, want ErrVaultOrphaned", err)
	}
	// Still the umbrella, so a caller that only tests for "the vault cannot be
	// opened" keeps working.
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("ErrVaultOrphaned must wrap ErrKeyringUnavailable; got %v", err)
	}

	// And the ciphertext is untouched, which is what makes recovery the user's
	// call rather than ours.
	entries, rerr := os.ReadDir(dir)
	if rerr != nil {
		t.Fatalf("read dir: %v", rerr)
	}
	if len(entries) != 1 {
		t.Errorf("want the value file left alone, found %d entries", len(entries))
	}
}

// The other side of the same check: an empty vault has nothing to orphan, so a
// first-ever use still mints a key and works.
func TestFirstUseMintsAKey(t *testing.T) {
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro", t.TempDir())

	if err := s.Set("env:global:TOKEN", "v"); err != nil {
		t.Fatalf("Set on a fresh vault: %v", err)
	}
	got, err := s.Get("env:global:TOKEN")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v" {
		t.Errorf("got %q, want %q", got, "v")
	}
}

// A malformed entry is not the same condition as a missing one, and the
// remediations are opposite: nothing is wrong with the machine, and recovering
// means accepting the values are gone. Callers get a distinct sentinel under the
// same umbrella.
func TestUnusableMasterKeyIsItsOwnCondition(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	if err := testStore(t, kr, "astro", dir).Set("env:global:TOKEN", "v"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := kr.Set("astro", keyringAccount, "!!!not-base64!!!"); err != nil {
		t.Fatalf("corrupt: %v", err)
	}

	_, err := testStore(t, kr, "astro", dir).Get("env:global:TOKEN")
	if !errors.Is(err, ErrMasterKeyUnusable) {
		t.Errorf("err = %v, want ErrMasterKeyUnusable", err)
	}
	if errors.Is(err, ErrVaultOrphaned) {
		t.Error("a malformed key is not an orphaned vault; the two need different remediations")
	}
}

// The platform's own reason has to survive into the error chain. Whether the
// keyring is permanently unavailable (an unsupported platform: stop asking,
// disable the feature) or transiently so (no dbus session yet: worth another try
// once the session is up) is the distinction that decides whether retrying costs
// an OS keyring round trip per entry — and only the wrapped chain carries it.
// Formatting it with %v looked identical and threw it away.
func TestKeyringFailureKeepsThePlatformReason(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	if err := testStore(t, kr, "astro", dir).Set("env:global:TOKEN", "v"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	kr.err = keyring.ErrUnsupportedPlatform
	_, err := testStore(t, kr, "astro", dir).Get("env:global:TOKEN")
	if !errors.Is(err, ErrKeyringUnavailable) {
		t.Errorf("err = %v, want it to wrap ErrKeyringUnavailable", err)
	}
	if !errors.Is(err, keyring.ErrUnsupportedPlatform) {
		t.Errorf("err = %v, want the platform's own error still reachable in the chain", err)
	}
}
