package plan

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// Environ carries what a start would inject, and a required value with no
// source leaves it out rather than refusing: the DAG that needs the value
// reports the problem itself.
func TestEnvironCarriesTheStartValuesAndSkipsMissingOnes(t *testing.T) {
	const m = `[project]
name = 'demo'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]

[tool.astro.env]
ASTRO_TEST_REQUIRED = {}
ASTRO_TEST_DEFAULTED = { default = 'from-manifest' }
`
	dir := isolatedProject(t, m, "ASTRO_TEST_REQUIRED", "ASTRO_TEST_DEFAULTED", "ASTRO_TEST_DOTENV", "ASTRONOMER_ENVIRONMENT")
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("ASTRO_TEST_DOTENV=sandbox\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	man, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	if err != nil {
		t.Fatal(err)
	}

	env, err := Environ(dir, man)
	if err != nil {
		t.Fatalf("a missing required value should not stop Environ: %v", err)
	}
	for _, want := range []string{"ASTRO_TEST_DOTENV=sandbox", "ASTRO_TEST_DEFAULTED=from-manifest", "ASTRONOMER_ENVIRONMENT=local"} {
		if !slices.Contains(env, want) {
			t.Errorf("Environ is missing %s", want)
		}
	}
	for _, kv := range env {
		if strings.HasPrefix(kv, "ASTRO_TEST_REQUIRED=") {
			t.Errorf("a value with no source should be left out, got %q", kv)
		}
	}
}
