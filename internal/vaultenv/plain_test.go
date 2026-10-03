package vaultenv

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// A plain writer stores and reads its value with no keyring at all, and a
// Source resolves and injects it like any other vault entry: a plain global
// reaches Airflow the way a secret one does.
func TestPlainWriterNeedsNoKeyring(t *testing.T) {
	keyring.MockInitWithError(errors.New("no Secret Service available"))
	t.Cleanup(keyring.MockInit)

	dir := filepath.Join(t.TempDir(), "secrets")
	store, err := secrets.NewKeyringStore(secrets.Config{Service: secrets.DefaultService, Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	w := &Writer{store: store, scope: secrets.GlobalScope, dir: dir, Plain: true, NewAutoLink: true}
	if _, err := w.Set(localenv.KindEnv, "REGION", "us-east-1"); err != nil {
		t.Fatalf("plain Set with no keyring: %v", err)
	}
	got, ok, err := w.Get(localenv.KindEnv, "REGION")
	if err != nil || !ok || got != "us-east-1" {
		t.Fatalf("Get = %q, %v, %v; want the plain value", got, ok, err)
	}
	metas, err := store.ListMeta()
	if err != nil || len(metas) != 1 || !metas[0].Plain {
		t.Fatalf("ListMeta = %+v, %v; want one plain entry", metas, err)
	}

	src := newSource(store, "")
	if inj := src.SecretInjection(); inj["REGION"] != "us-east-1" {
		t.Fatalf("SecretInjection = %v, want the plain global", inj)
	}

	// The default writer still encrypts, so with no keyring it refuses.
	secret := &Writer{store: store, scope: secrets.GlobalScope, dir: dir, NewAutoLink: true}
	if _, err := secret.Set(localenv.KindEnv, "TOKEN", "x"); !errors.Is(err, secrets.ErrKeyringUnavailable) {
		t.Fatalf("secret Set with no keyring = %v, want ErrKeyringUnavailable", err)
	}
}
