package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeRaw puts a value file on disk the way a writer to the directory could,
// bypassing the store.
func writeRaw(t *testing.T, path string, vf valueFile) {
	t.Helper()
	raw, err := json.Marshal(vf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func readRaw(t *testing.T, path string) valueFile {
	t.Helper()
	vf, err := readValueFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return vf
}

// The attack the envelope exists to stop: someone who can write the directory
// but does not have the master key plants a value for a secret entry. It must
// be refused, not served as that secret.
func TestPlantedPlaintextIsRefused(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	const key = "conn:global:warehouse"
	// A real secret first, so the vault has a master key and is in use.
	if err := s.Set("env:global:OTHER", "x"); err != nil {
		t.Fatal(err)
	}

	for name, planted := range map[string]string{
		"new entry":          key,
		"over a real secret": "env:global:OTHER",
	} {
		t.Run(name, func(t *testing.T) {
			writeRaw(t, s.path(planted), valueFile{Key: planted, Value: "postgres://attacker@evil.example/db"})
			got, err := s.Get(planted)
			if !errors.Is(err, ErrUnencrypted) {
				t.Errorf("Get(planted) err = %v, want ErrUnencrypted", err)
			}
			if got != "" {
				t.Errorf("Get(planted) returned %d bytes, want none", len(got))
			}
		})
	}
}

// A ciphertext lifted from one entry and written into another's file, with
// the key field rewritten to match, must not open as the second entry.
func TestSwappedCiphertextIsRefused(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	const a, b = "conn:global:prod", "conn:global:dev"
	if err := s.Set(a, "prod-credential"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set(b, "dev-credential"); err != nil {
		t.Fatal(err)
	}
	stolen := readRaw(t, s.path(a))
	writeRaw(t, s.path(b), valueFile{Key: b, Value: stolen.Value})

	got, err := s.Get(b)
	if !errors.Is(err, ErrTampered) {
		t.Errorf("Get(b) err = %v, want ErrTampered", err)
	}
	if got != "" {
		t.Errorf("Get(b) returned %d bytes, want none", len(got))
	}
}

// The same swap without rewriting the key field: the file under b's name says
// it is a's. The store must not return it for b, plain or secret.
func TestFileNamingAnotherKeyIsRefused(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	const a, b = "conn:global:prod", "conn:global:dev"
	if err := s.Set(a, "prod-credential"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlain("env:global:REGION", "us-east-1"); err != nil {
		t.Fatal(err)
	}
	for src, dst := range map[string]string{a: b, "env:global:REGION": "env:global:ZONE"} {
		raw, err := os.ReadFile(s.path(src))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.path(dst), raw, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := s.Get(dst)
		if !errors.Is(err, ErrTampered) {
			t.Errorf("Get(%s) err = %v, want ErrTampered", dst, err)
		}
		if got != "" {
			t.Errorf("Get(%s) returned %d bytes, want none", dst, len(got))
		}
	}
}

// A value sealed under another master key says so, distinctly from a damaged
// one, so a caller can report a changed key rather than N corrupt entries.
func TestDifferentMasterKeyIsItsOwnError(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	if err := testStore(t, kr, "astro-test", dir).Set("env:global:A", "v"); err != nil {
		t.Fatal(err)
	}
	other := make([]byte, keyBytes)
	other[0] = 1
	if err := kr.Set("astro-test", keyringAccount, base64.StdEncoding.EncodeToString(other)); err != nil {
		t.Fatal(err)
	}
	_, err := testStore(t, kr, "astro-test", dir).Get("env:global:A")
	if !errors.Is(err, ErrWrongMasterKey) {
		t.Fatalf("err = %v, want ErrWrongMasterKey", err)
	}
	if errors.Is(err, ErrTampered) || errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("err = %v: a different key is per entry and not tampering", err)
	}
}

func TestEmptyValueIsStoredEncrypted(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	if err := s.Set("env:global:EMPTY", ""); err != nil {
		t.Fatal(err)
	}
	if v := readRaw(t, s.path("env:global:EMPTY")).Value; !strings.HasPrefix(v, encPrefixV2) {
		t.Fatalf("empty secret stored as %q, want a v2 envelope", v)
	}
	got, err := s.Get("env:global:EMPTY")
	if err != nil || got != "" {
		t.Fatalf("Get = err %v, empty=%v", err, got == "")
	}
}

func TestSetWritesTheCurrentEnvelope(t *testing.T) {
	s := testStore(t, newFakeKeyring(), "astro-test", t.TempDir())
	if err := s.Set("env:global:A", "v"); err != nil {
		t.Fatal(err)
	}
	if v := readRaw(t, s.path("env:global:A")).Value; !strings.HasPrefix(v, encPrefixV2) {
		t.Fatalf("Set wrote %.7q..., want %q", v, encPrefixV2)
	}
}

// A vault an older build wrote: v1 values and a bare empty string. Get reads
// both, and Upgrade turns both into v2.
func TestUpgradeRewritesTheOlderForm(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro-test", dir)
	c, err := s.aead()
	if err != nil {
		t.Fatal(err)
	}
	const old, empty, current, plain = "env:global:OLD", "env:global:EMPTY", "env:global:NEW", "env:global:PLAIN"
	writeRaw(t, s.path(old), valueFile{Key: old, Value: sealV1(t, c, "old-value")})
	writeRaw(t, s.path(empty), valueFile{Key: empty, Value: ""})
	if err := s.Set(current, "new-value"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlain(plain, "p"); err != nil {
		t.Fatal(err)
	}
	currentBefore := readRaw(t, s.path(current))

	if got, err := s.Get(old); err != nil || got != "old-value" {
		t.Fatalf("Get(v1) before upgrade: err %v, matched=%v", err, got == "old-value")
	}
	if got, err := s.Get(empty); err != nil || got != "" {
		t.Fatalf("Get(bare empty) before upgrade: err %v, %d bytes; want the empty value", err, len(got))
	}

	report, err := Upgrade(s)
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if report.Upgraded != 2 || len(report.Skipped) != 0 {
		t.Fatalf("report = %d upgraded, %d skipped; want 2, 0", report.Upgraded, len(report.Skipped))
	}
	for _, k := range []string{old, empty} {
		if v := readRaw(t, s.path(k)).Value; !strings.HasPrefix(v, encPrefixV2) {
			t.Errorf("%s after upgrade is not v2", k)
		}
	}
	if got := readRaw(t, s.path(current)); got != currentBefore {
		t.Error("Upgrade rewrote a value that was already current")
	}
	if got := readRaw(t, s.path(plain)); !got.Plain || got.Value != "p" {
		t.Error("Upgrade touched a plain entry")
	}
	for k, want := range map[string]string{old: "old-value", empty: "", current: "new-value"} {
		if got, err := s.Get(k); err != nil || got != want {
			t.Errorf("Get(%s) after upgrade: err %v, matched=%v", k, err, got == want)
		}
	}
}

// What Upgrade cannot open it leaves exactly as found and reports, and it
// never upgrades a file that is not stored under its own key's name.
func TestUpgradeSkipsWhatItCannotOpen(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	c, err := s.aead()
	if err != nil {
		t.Fatal(err)
	}
	foreign := testCipher(t, 0xaa)
	const bad, misplaced, good = "env:global:BAD", "env:global:MISPLACED", "env:global:GOOD"
	writeRaw(t, s.path(bad), valueFile{Key: bad, Value: sealV1(t, foreign, "x")})
	writeRaw(t, s.path("env:global:ELSEWHERE"), valueFile{Key: misplaced, Value: sealV1(t, c, "y")})
	writeRaw(t, s.path(good), valueFile{Key: good, Value: sealV1(t, c, "z")})
	badBefore, _ := os.ReadFile(s.path(bad))
	misplacedBefore, _ := os.ReadFile(s.path("env:global:ELSEWHERE"))

	report, err := s.Upgrade()
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}
	if report.Upgraded != 1 || len(report.Skipped) != 2 {
		t.Fatalf("report = %d upgraded, %d skipped; want 1, 2", report.Upgraded, len(report.Skipped))
	}
	skipped := map[string]error{}
	for _, sk := range report.Skipped {
		skipped[sk.Key] = sk.Err
	}
	if skipped[bad] == nil {
		t.Errorf("undecryptable entry not reported: %v", report.Skipped)
	}
	if !errors.Is(skipped[misplaced], ErrTampered) {
		t.Errorf("misplaced entry: %v, want ErrTampered", skipped[misplaced])
	}
	if after, _ := os.ReadFile(s.path(bad)); !bytes.Equal(after, badBefore) {
		t.Error("Upgrade changed an entry it could not open")
	}
	if after, _ := os.ReadFile(s.path("env:global:ELSEWHERE")); !bytes.Equal(after, misplacedBefore) {
		t.Error("Upgrade changed a misplaced entry")
	}
}

// A vault with nothing to upgrade is checked from the files alone: no keyring
// call, so calling Upgrade on every start never prompts.
func TestUpgradeWithNothingToDoNeverTouchesKeyring(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	writer := testStore(t, kr, "astro-test", dir)
	if err := writer.Set("env:global:A", "v"); err != nil {
		t.Fatal(err)
	}
	if err := writer.SetPlain("env:global:B", "p"); err != nil {
		t.Fatal(err)
	}
	dead := newFakeKeyring()
	dead.err = errors.New("no dbus")
	for _, d := range []string{dir, filepath.Join(t.TempDir(), "missing")} {
		report, err := testStore(t, dead, "astro-test", d).Upgrade()
		if err != nil || report.Upgraded != 0 || len(report.Skipped) != 0 {
			t.Fatalf("Upgrade on a current vault = %+v, %v; want nothing", report, err)
		}
	}
	if dead.gets != 0 || dead.sets != 0 {
		t.Fatalf("Upgrade touched the keyring: %d gets, %d sets", dead.gets, dead.sets)
	}
}

func TestUpgradeOnAStoreWithoutOne(t *testing.T) {
	if r, err := Upgrade(notPlain{}); err != nil || r.Upgraded != 0 {
		t.Fatalf("Upgrade(non-upgrader) = %+v, %v; want a no-op", r, err)
	}
}
