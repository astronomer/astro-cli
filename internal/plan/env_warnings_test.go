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
// reported, and does not stop the start.
//
// The manifest is self-contained: the default resolves the name, so the value is
// present — nothing missing, so no gate fires — and wrong, since 99999 is not a
// port.
func TestBuildReportsAValueWarningWithoutRefusing(t *testing.T) {
	dir := isolatedProject(t, `[project]
name = 'demo'
requires-python = '>=3.10'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

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
	// The message names the offending value, since the caller prints this
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
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

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

// valueWarnings' exclusion, tested at its own level: Build cannot reach it with
// a missing violation, because the Missing gate returns first. It matters if a
// start ever proceeds despite missing values, where the filter is what stops an
// error and a duplicate warning naming the same value.
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
// source the resolver would otherwise consult: HOME and USERPROFILE so
// `~/.astro/env` cannot contribute, XDG_CACHE_HOME so real user state cannot,
// and the declared names themselves, which shell env would otherwise satisfy
// above the manifest default.
//
// The fixtures are ASTRO_TEST_-prefixed for the same reason: a name a shell is
// likely to export, PORT among them, makes the result depend on the developer's
// environment.
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
// t.Setenv can only set, and setting a name to "" is not absence — an empty
// value still resolves, which is "present" to the gate and skipped by the type
// check. So the original is registered for restoration via t.Setenv and then
// unset outright.
//
// Shell env outranks a manifest default, so any name a test relies on
// defaulting has to be cleared.
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
