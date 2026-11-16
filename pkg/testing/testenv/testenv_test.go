package testenv

import (
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/secrets"
	shared "github.com/astronomer/astro-cli/pkg/testenv"
)

func TestMain(m *testing.M) { Main(m) }

// The root's test binaries get the in-memory keyring: a value set here reads
// back, and the OS keychain was never asked.
func TestTheRootGetsTheInMemoryKeyring(t *testing.T) {
	if err := keyring.Set("astro-testenv", "probe", "value"); err != nil {
		t.Fatalf("keyring.Set: %v", err)
	}
	if got, err := keyring.Get("astro-testenv", "probe"); err != nil || got != "value" {
		t.Errorf("keyring.Get = %q, %v", got, err)
	}
	if n := secrets.OSKeyringRefusals(); n != 0 {
		t.Errorf("%d OS keychain refusals", n)
	}
}

// The guard reads CI by the rule pkg/input uses to decide a run cannot ask: the
// two answer the same for every spelling. pkg/testenv cannot import pkg/input
// (it is a production package), so the rule is written twice and this holds the
// two together.
func TestTheGuardReadsCIAsInputDoes(t *testing.T) {
	for _, v := range []string{"", "0", "false", "FALSE", " false ", " 0 ", "true", "1", "yes", "no", "TRUE", "  "} {
		t.Setenv("CI", v)
		// With the other CI markers cleared by Isolate and a test binary counting
		// as a terminal, CI is the only thing that makes Refusal refuse.
		if got, refused := shared.CIIsSet(), input.Refusal(false, nil) != ""; got != refused {
			t.Errorf("CI=%q: the guard says %v, input.Refusal says %v", v, got, refused)
		}
	}
}
