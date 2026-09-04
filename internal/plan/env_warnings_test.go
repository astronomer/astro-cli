package plan

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/localrt"
)

// A value that resolved to something its declaration did not promise is
// reported, and does NOT stop the start.
//
// The manifest is self-contained on purpose: the default resolves the name, so
// the value is present (nothing is missing, so no gate fires) and wrong (99999
// is not a port). That is precisely the combination the old behavior dropped on
// the floor — computed into res.Violations and surfaced nowhere.
func TestBuildReportsAValueWarningWithoutRefusing(t *testing.T) {
	dir := isolatedProject(t, `[project]
name = 'demo'
requires-python = '>=3.10'

[tool.astro]
airflow = '3.1'

[tool.astro.env]
ASTRO_TEST_PORT = { type = 'port', default = '99999' }
ASTRO_TEST_FINE = { type = 'int', default = '3' }
`, "ASTRO_TEST_PORT", "ASTRO_TEST_FINE")

	built, err := Build(dir, Options{Mode: localrt.ModeStandalone})
	if err != nil {
		t.Fatalf("a wrong-typed value must not refuse the start: %v", err)
	}
	if len(built.EnvWarnings) != 1 {
		t.Fatalf("want exactly one warning, got %+v", built.EnvWarnings)
	}
	w := built.EnvWarnings[0]
	if w.Key != "ASTRO_TEST_PORT" || w.Kind != envschema.ViolationWrongType || w.Section != envschema.SectionEnvVar {
		t.Errorf("unexpected warning: %+v", w)
	}
	// The message has to name the offending value: the caller prints this
	// without the value to hand.
	if !strings.Contains(w.Reason, "99999") {
		t.Errorf("reason %q does not name the offending value", w.Reason)
	}
}

// The conforming case stays silent, so a clean project prints no warning noise.
func TestBuildReportsNoWarningsWhenValuesConform(t *testing.T) {
	dir := isolatedProject(t, `[project]
name = 'demo'
requires-python = '>=3.10'

[tool.astro]
airflow = '3.1'

[tool.astro.env]
ASTRO_TEST_PORT = { type = 'port', default = '8080' }
`, "ASTRO_TEST_PORT")

	built, err := Build(dir, Options{Mode: localrt.ModeStandalone})
	if err != nil {
		t.Fatal(err)
	}
	if len(built.EnvWarnings) != 0 {
		t.Errorf("want no warnings, got %+v", built.EnvWarnings)
	}
}

// valueWarnings' exclusion, tested at its own level.
//
// Build cannot currently reach it with a missing violation — the Missing gate
// returns first — so exercising it through Build is impossible and this is the
// only place the behavior is pinned. It matters for the day a start proceeds
// despite missing values: without the filter the user would get an error and a
// duplicate warning naming the same value.
func TestValueWarningsExcludesMissing(t *testing.T) {
	in := []envschema.Violation{
		{Kind: envschema.ViolationMissing, Section: envschema.SectionEnvVar, Key: "GONE"},
		{Kind: envschema.ViolationWrongType, Section: envschema.SectionEnvVar, Key: "WRONG", Reason: "bad"},
		{Kind: envschema.ViolationMissing, Section: envschema.SectionConnection, Key: "ALSO_GONE"},
	}
	var keys []string
	for _, v := range valueWarnings(in) {
		keys = append(keys, v.Key)
	}
	if !reflect.DeepEqual(keys, []string{"WRONG"}) {
		t.Errorf("valueWarnings = %v, want only the non-missing finding", keys)
	}
}

func TestValueWarningsOnNothing(t *testing.T) {
	if got := valueWarnings(nil); got != nil {
		t.Errorf("valueWarnings(nil) = %+v, want nil", got)
	}
	only := []envschema.Violation{{Kind: envschema.ViolationMissing, Key: "GONE"}}
	if got := valueWarnings(only); got != nil {
		t.Errorf("valueWarnings(missing only) = %+v, want nil", got)
	}
}

// isolatedProject writes a manifest into a fresh dir and cuts every ambient
// source the resolver would otherwise consult, following plan_test.go's
// pattern: HOME and USERPROFILE so `~/.astro/env` cannot contribute, and
// XDG_CACHE_HOME so real user state cannot.
//
// The fixture names are ASTRO_TEST_-prefixed for the same reason. These tests
// first used PORT, which many dev shells and CI runners export — and the chain
// puts shell env above the manifest default, so `PORT=not-a-port go test`
// failed them with the developer's own environment. A test that depends on what
// is NOT set in your shell is not a test.
func isolatedProject(t *testing.T, manifest string, declared ...string) string {
	t.Helper()
	clearEnv(t, declared...)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.Marker), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return dir
}

// clearEnv removes names from the process environment for the duration of the
// test, restoring whatever was there afterwards.
//
// t.Setenv can only set, and setting a fixture name to "" is not the same as
// its being absent — an empty value still resolves, which is "present" to the
// gate and skipped by the type check. So the original is registered for
// restoration via t.Setenv and then unset outright.
//
// Renaming the fixtures away from PORT lowered the odds of a collision; this
// removes them. Shell env legitimately outranks a manifest default, so ANY
// fixture name a test relies on defaulting must be cleared, not just an
// unlucky one.
func clearEnv(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if old, ok := os.LookupEnv(n); ok {
			t.Setenv(n, old) // registers the restore
		}
		if err := os.Unsetenv(n); err != nil {
			t.Fatal(err)
		}
	}
}
