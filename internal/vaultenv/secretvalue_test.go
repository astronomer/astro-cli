package vaultenv

import (
	"testing"

	"github.com/astronomer/astro-cli/pkg/scaffold"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// The conversion asks whether the vault's value is the one it carries, so an
// equal value can be carried instead of kept apart.
func TestHasSecretValueComparesTheHeldValue(t *testing.T) {
	store := testVault(t)
	scope := t.TempDir()
	put(t, store, secrets.KindVar, scope, "api_token", "set-by-hand")
	w := &Writer{store: store, scope: scope}
	var _ scaffold.SecretValueChecker = w

	cases := []struct {
		name, key, value string
		kind             secrets.Kind
		want             scaffold.SecretHeld
	}{
		{name: "equal, another spelling", kind: secrets.KindVar, key: "API_TOKEN", value: "set-by-hand", want: scaffold.SecretHeld{Held: true, Compared: true, Equal: true}},
		{name: "different", kind: secrets.KindVar, key: "api_token", value: "from-the-file", want: scaffold.SecretHeld{Held: true, Compared: true}},
		{name: "not held", kind: secrets.KindConn, key: "api_token", value: "set-by-hand"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := w.HasSecretValue(tc.kind, tc.key, tc.value)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Errorf("HasSecretValue = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// An equal value under another spelling is already in this vault, so storing
// it is skipped rather than adding a second entry for the same env key.
func TestSetSecretSkipsAnEqualValueUnderAnotherSpelling(t *testing.T) {
	store := testVault(t)
	scope := t.TempDir()
	put(t, store, secrets.KindVar, scope, "api_token", "same")
	w := &Writer{store: store, scope: scope}

	if err := w.SetSecret(secrets.KindVar, "API_TOKEN", "same"); err != nil {
		t.Fatal(err)
	}
	metas, err := store.ListMeta()
	if err != nil {
		t.Fatal(err)
	}
	if len(metas) != 1 {
		t.Fatalf("vault holds %d entries, want the one api_token", len(metas))
	}

	// A value not held is still stored.
	if err := w.SetSecret(secrets.KindVar, "other", "v"); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := w.Get(localKind(secrets.KindVar), "other"); err != nil || !ok || got != "v" {
		t.Fatalf("Get(other) = %q, %v, %v", got, ok, err)
	}
}
