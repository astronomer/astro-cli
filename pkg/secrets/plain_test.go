package secrets

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// A plain entry is written, read and listed without the keyring: the keyring
// here fails every call and counts them.
func TestSetPlainNeverTouchesKeyring(t *testing.T) {
	kr := newFakeKeyring()
	kr.err = errors.New("no dbus")
	s := testStore(t, kr, "astro-test", t.TempDir())

	if err := SetPlain(s, "env:global:REGION", "us-east-1"); err != nil {
		t.Fatalf("SetPlain: %v", err)
	}
	got, err := s.Get("env:global:REGION")
	if err != nil || got != "us-east-1" {
		t.Fatalf("Get = %q, %v; want the plain value", got, err)
	}
	metas, err := s.ListMeta()
	if err != nil || len(metas) != 1 || !metas[0].Plain || metas[0].Key != "env:global:REGION" {
		t.Fatalf("ListMeta = %+v, %v; want one plain entry", metas, err)
	}
	if kr.gets != 0 || kr.sets != 0 {
		t.Fatalf("plain entry touched the keyring: %d gets, %d sets", kr.gets, kr.sets)
	}
}

// Set and SetPlain replace each other, so one key is one entry, and the flag
// follows the last write.
func TestSetAndSetPlainReplaceEachOther(t *testing.T) {
	s := testStore(t, newFakeKeyring(), "astro-test", t.TempDir())
	const key = "var:global:region"
	if err := s.Set(key, "secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlain(key, "plain"); err != nil {
		t.Fatal(err)
	}
	metas, _ := s.ListMeta()
	if len(metas) != 1 || !metas[0].Plain {
		t.Fatalf("after SetPlain: %+v, want one plain entry", metas)
	}
	if got, _ := s.Get(key); got != "plain" {
		t.Fatalf("Get = %q, want plain", got)
	}
	if err := s.Set(key, "secret2"); err != nil {
		t.Fatal(err)
	}
	metas, _ = s.ListMeta()
	if len(metas) != 1 || metas[0].Plain {
		t.Fatalf("after Set: %+v, want one secret entry", metas)
	}
	if got, _ := s.Get(key); got != "secret2" {
		t.Fatalf("Get = %q, want secret2", got)
	}
}

// A secret's file carries no marker, and its value is ciphertext.
func TestSecretFileHasNoPlainMarker(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	if err := s.Set("env:global:TOKEN", "hunter2"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(s.path("env:global:TOKEN"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "plain") || strings.Contains(string(raw), "hunter2") {
		t.Fatalf("secret file = %s, want ciphertext and no plain marker", raw)
	}
}

// A file written before the marker existed reads as secret: decrypted with the
// key, and listed with Plain false.
func TestUnmarkedFileReadsAsSecret(t *testing.T) {
	dir := t.TempDir()
	kr := newFakeKeyring()
	s := testStore(t, kr, "astro-test", dir)
	gcm, err := s.aead()
	if err != nil {
		t.Fatal(err)
	}
	enc, err := encrypt(gcm, "old-value")
	if err != nil {
		t.Fatal(err)
	}
	legacy := `{"key":"env:global:OLD","value":"` + enc + `"}`
	if err := os.WriteFile(s.path("env:global:OLD"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get("env:global:OLD")
	if err != nil || got != "old-value" {
		t.Fatalf("Get = %q, %v; want the decrypted value", got, err)
	}
	metas, _ := s.ListMeta()
	if len(metas) != 1 || metas[0].Plain {
		t.Fatalf("ListMeta = %+v, want one secret entry", metas)
	}
}

// Plain entries alone are not an orphaned vault: nothing was encrypted, so a
// first secret may mint the key.
func TestPlainEntriesDoNotOrphanTheVault(t *testing.T) {
	dir := t.TempDir()
	s := testStore(t, newFakeKeyring(), "astro-test", dir)
	if err := s.SetPlain("env:global:A", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("env:global:B", "2"); err != nil {
		t.Fatalf("Set beside only plain entries = %v, want a minted key", err)
	}
	// With a secret on disk and the key gone, the vault is orphaned again.
	fresh := testStore(t, newFakeKeyring(), "astro-test", dir)
	if err := fresh.Set("env:global:C", "3"); !errors.Is(err, ErrVaultOrphaned) {
		t.Fatalf("Set with the key gone = %v, want ErrVaultOrphaned", err)
	}
}

type notPlain struct{ Store }

func TestSetPlainUnsupported(t *testing.T) {
	if err := SetPlain(notPlain{}, "k", "v"); !errors.Is(err, ErrPlainUnsupported) {
		t.Fatalf("SetPlain = %v, want ErrPlainUnsupported", err)
	}
}
