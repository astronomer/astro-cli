package vaultenv

import (
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
)

// pkg/secrets keeps these three apart deliberately, and says why: "Saying 'os
// keyring unavailable' alone would send someone to check a daemon that is
// running fine." Both specific sentinels WRAP the umbrella, so a single
// errors.Is on it collapses all three into advice that is wrong for two.
func TestRefusalTellsTheThreeConditionsApart(t *testing.T) {
	orphaned := refusal(secrets.ErrVaultOrphaned).Error()
	unusable := refusal(secrets.ErrMasterKeyUnusable).Error()
	unreachable := refusal(secrets.ErrKeyringUnavailable).Error()

	// An orphaned vault is not a keyring problem: unlocking will never help,
	// because the key is gone and the values are unrecoverable.
	//
	// Asserted on the GUIDANCE, not on words that could arrive from the wrapped
	// sentinel: "gone" is in ErrVaultOrphaned's own text, so matching it passed
	// even when this branch collapsed into the umbrella message.
	if strings.Contains(orphaned, "headless") || strings.Contains(orphaned, "unlock") {
		t.Errorf("orphaned vault was given machine-level advice that cannot help:\n%s", orphaned)
	}
	if !strings.Contains(orphaned, "cannot be recovered") {
		t.Errorf("orphaned vault should say the values are unrecoverable:\n%s", orphaned)
	}
	if !strings.Contains(orphaned, ".astro/secrets") {
		t.Errorf("orphaned vault should name the directory whose deletion clears it:\n%s", orphaned)
	}
	// Nothing about the machine is wrong for an unusable key, so there is no
	// daemon to go and check.
	if strings.Contains(unusable, "headless") || strings.Contains(unusable, "CI") {
		t.Errorf("unusable key pointed at a machine problem:\n%s", unusable)
	}
	// The umbrella is the one where the CI advice belongs.
	if !strings.Contains(unreachable, "environment") {
		t.Errorf("unreachable keyring should say what to do instead:\n%s", unreachable)
	}

	if orphaned == unreachable || unusable == unreachable || orphaned == unusable {
		t.Error("two conditions produced the same message, so one of them is being given the wrong remediation")
	}
	// And an unrelated error passes through untouched.
	plain := errors.New("disk full")
	if got := refusal(plain); got != plain {
		t.Errorf("refusal rewrote an unrelated error: %v", got)
	}
}

// A conversion asks about a name as the 1.x file spells it. A value stored under
// another spelling of the same Airflow key is the one Airflow reads, so it
// counts as held.
func TestHasSecretMatchesOnTheEnvKey(t *testing.T) {
	store := testVault(t)
	scope := t.TempDir()
	put(t, store, secrets.KindVar, scope, "api_token", "set-by-hand")
	w := &Writer{store: store, scope: scope}

	held, err := w.HasSecret(secrets.KindVar, "API_TOKEN")
	if err != nil || !held {
		t.Fatalf("HasSecret(API_TOKEN) = %v, %v; want true for a value stored as api_token", held, err)
	}
	held, err = w.HasSecret(secrets.KindConn, "API_TOKEN")
	if err != nil || held {
		t.Fatalf("HasSecret for another kind = %v, %v; want false", held, err)
	}
}
