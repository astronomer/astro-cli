package vaultenv

import (
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/envresolve"
	"github.com/astronomer/astro-cli/internal/localenv"
	"github.com/astronomer/astro-cli/pkg/secrets"
)

// A Variable an older build stored under a key with a leading digit no longer
// resolves. It is listed with the reason, diagnosed, and still deletable.
func TestALeadingDigitVariableIsListedAndDeletable(t *testing.T) {
	store := testVault(t)
	scope := t.TempDir()
	put(t, store, secrets.KindVar, scope, "1st_region", "us-east-1")

	s := newSource(store, scope)
	var found *localenv.VaultEntry
	for _, tier := range s.Tiers() {
		for i := range tier.Entries {
			if tier.Entries[i].Name == "1st_region" {
				found = &tier.Entries[i]
			}
		}
	}
	if found == nil {
		t.Fatal("1st_region is not listed")
	}
	if found.EnvKey != "AIRFLOW_VAR_1ST_REGION" || !strings.Contains(found.Invalid, "not a valid Airflow Variable key") ||
		!strings.Contains(found.Invalid, "rename or delete it") {
		t.Errorf("entry = %+v, want its stored key and the reason", *found)
	}
	items, err := localenv.List(nil, "", nil, localenv.ListOptions{VaultTiers: s.Tiers()})
	if err != nil {
		t.Fatal(err)
	}
	listed := false
	for _, it := range items {
		if it.Name == "1st_region" {
			listed = it.Invalid == found.Invalid && it.RemoveHint != "" && it.DeclareHint == ""
		}
	}
	if !listed {
		t.Errorf("list rows = %+v, want 1st_region with the reason and a remove hint", items)
	}
	if why := envresolve.Diagnose(s.Providers()[0], "AIRFLOW_VAR_1ST_REGION"); !strings.Contains(why, "1st_region") || !strings.Contains(why, "rename or delete it") {
		t.Errorf("Diagnose = %q, want the reason", why)
	}

	w := &Writer{store: store, scope: scope}
	ok, err := w.Delete(localenv.KindVar, "1st_region")
	if err != nil || !ok {
		t.Fatalf("Delete(1st_region) = %v, %v; want it removed", ok, err)
	}
	if metas, _ := store.ListMeta(); len(metas) != 0 {
		t.Errorf("vault still holds %v", metas)
	}
}
