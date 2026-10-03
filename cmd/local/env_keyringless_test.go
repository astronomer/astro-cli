package local

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/vaultenv"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// A fresh machine with no keyring at all: no ~/.astro/secrets yet, and a
// keyring that fails every call. The first plain global creates the vault,
// stores the value unencrypted, and is read back by get and by a start's
// resolution, none of which needs the keyring. An encrypted set on the same
// machine refuses and names --plain.
//
// The keyring mock fails every call, so any path that reached it would fail
// the command; pkg/secrets' TestFirstPlainWriteOnAFreshMachineNeedsNoKeyring
// counts the calls (zero) on the store underneath.
func TestAFreshMachineWithNoKeyringStoresAPlainGlobal(t *testing.T) {
	dir := secretEnvProject(t, "")
	keyring.MockInitWithError(errors.New("no Secret Service available"))
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	vaultDir := filepath.Join(home, ".astro", "secrets")
	if _, err := os.Stat(vaultDir); !os.IsNotExist(err) {
		t.Fatalf("the test machine already has a vault: %v", err)
	}

	_, stderr := mustRun(t, dir, "variable", "set", "REGION", "--value", "us-east-1", "--global", "--plain")
	if strings.Contains(strings.ToLower(stderr), "keyring") {
		t.Errorf("a plain global set mentioned the keyring: %q", stderr)
	}
	if fi, err := os.Stat(vaultDir); err != nil || !fi.IsDir() {
		t.Fatalf("the first set did not create the vault: %v", err)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Fatalf("vault holds %d entries, want the one plain global", n)
	}

	if got := getJSON(t, dir, "REGION", "--global"); got.Value != "us-east-1" || got.Source != vaultenv.SourceGlobal {
		t.Errorf("get --global = %+v, want the plain global", got)
	}

	// A new global reaches no project until linked; linking writes only the
	// link index, which needs no keyring either.
	mustRun(t, dir, "variable", "link", "REGION")
	if got := getJSON(t, dir, "REGION"); got.Value != "us-east-1" {
		t.Errorf("resolved get = %+v, want the plain global", got)
	}
	built, err := plan.Build(dir, plan.Options{Mode: localrt.ModeStandalone})
	if err != nil {
		t.Fatalf("a start's resolution without a keyring: %v", err)
	}
	if got := built.Plan.SecretEnv["REGION"]; got != "us-east-1" {
		t.Errorf("start resolves REGION = %q, want the plain global", got)
	}

	_, _, err = run(t, dir, "variable", "set", "TOKEN", "--value", "s3cr3t", "--global")
	if err == nil || !strings.Contains(err.Error(), "--plain") {
		t.Errorf("an encrypted set with no keyring = %v, want a refusal naming --plain", err)
	}
	if n := len(vaultFiles(t)); n != 1 {
		t.Errorf("the refused set left %d vault entries, want the one plain global", n)
	}
}
