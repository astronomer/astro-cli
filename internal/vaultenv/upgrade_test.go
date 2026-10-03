package vaultenv

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// A secret set from the CLI upgrades the values an older build left in the
// vault, so a CLI-only user's vault converges on the current envelope without
// a separate step.
func TestSetUpgradesOlderValues(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "secrets")
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	w := &Writer{store: store, scope: secrets.GlobalScope, label: SourceGlobal, dir: dir, NewAutoLink: true}
	if _, err := w.Set(localenv.KindEnv, "FIRST", "1"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// Plant a v1 value the way an older build wrote it, under the same key.
	encoded, err := keyring.Get(secrets.DefaultService, "master-key")
	if err != nil {
		t.Fatal(err)
	}
	master, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(master)
	if err != nil {
		t.Fatal(err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	oldKey, err := secrets.Key(secrets.KindEnv, secrets.GlobalScope, "OLD")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(oldKey))
	oldPath := filepath.Join(dir, hex.EncodeToString(sum[:])+".json")
	v1 := "enc:v1:" + base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte("old"), nil))
	raw, err := json.Marshal(map[string]string{"key": oldKey, "value": v1})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := w.Set(localenv.KindEnv, "SECOND", "2"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	after, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatal(err)
	}
	var vf struct{ Value string }
	if err := json.Unmarshal(after, &vf); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(vf.Value, "enc:v2:") {
		t.Fatalf("older value after a Set starts %.7q, want enc:v2:", vf.Value)
	}
	if got, err := store.Get(oldKey); err != nil || got != "old" {
		t.Fatalf("Get(upgraded): err %v, matched=%v", err, got == "old")
	}
}

// The integrity conditions each get advice of their own, not the keyring
// umbrella's: none of them is a machine problem.
func TestRefusalNamesIntegrityConditions(t *testing.T) {
	for name, err := range map[string]error{
		"unsafe dir": secrets.ErrVaultDirUnsafe,
		"wrong key":  secrets.ErrWrongMasterKey,
		"tampered":   secrets.ErrUnencrypted,
	} {
		msg := refusal(err).Error()
		if strings.Contains(msg, "headless") || strings.Contains(msg, "unlock") {
			t.Errorf("%s was given keyring advice:\n%s", name, msg)
		}
		if !errors.Is(refusal(err), err) {
			t.Errorf("%s: refusal dropped the sentinel", name)
		}
	}
}
