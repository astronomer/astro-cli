package local

import (
	"context"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

func TestMissingModuleReadsTheFinalException(t *testing.T) {
	for _, tc := range []struct {
		name, message, want string
	}{
		{"plain", "Traceback (most recent call last):\n  File \"dags/a.py\", line 1\nModuleNotFoundError: No module named 'pandas'", "pandas"},
		{"submodule", "ModuleNotFoundError: No module named 'google.cloud'", "google.cloud"},
		{"chained, ends in another error", "ModuleNotFoundError: No module named 'ujson'\n\nDuring handling...\nNameError: name 'x' is not defined", ""},
		{"chained, ends in the missing module", "ImportError: cannot import name 'x'\n\nDuring handling...\nModuleNotFoundError: No module named 'pandas'", "pandas"},
		{"no exception", "something odd", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingModule(tc.message); got != tc.want {
				t.Errorf("missingModule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRuntimeHintNamesThePackageTheImageShips(t *testing.T) {
	d, _ := testDeps(t)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return constraintsCatalog(t) }
	d.RuntimeFreeze = func(_ context.Context, build, python string) ([]string, error) {
		if build != "3.3-9" || python != "3.14" {
			t.Errorf("freeze asked for %s/%s, want 3.3-9/3.14", build, python)
		}
		return []string{"pandas==2.3.3", "psycopg2-binary==2.9.12", "PyYAML==6.0.3"}, nil
	}
	c := &cli{d: d}
	m, err := manifest.Parse([]byte(buildManifest))
	if err != nil {
		t.Fatal(err)
	}
	res := checks.Result{Findings: []checks.Finding{
		{Kind: checks.KindImportError, Message: "ModuleNotFoundError: No module named 'pandas'"},
		{Kind: checks.KindImportError, Message: "ModuleNotFoundError: No module named 'yaml'"},
		{Kind: checks.KindImportError, Message: "ModuleNotFoundError: No module named 'psycopg2'"},
		{Kind: checks.KindImportError, Message: "SyntaxError: invalid syntax"},
	}}
	got := c.withRuntimeHints(context.Background(), t.TempDir(), m, res).Findings

	want := "Astro Runtime 3.3-9 ships pandas 2.3.3, but this project doesn't list it: uv add 'pandas==2.3.3'"
	if got[0].Hint != want {
		t.Errorf("hint = %q, want %q", got[0].Hint, want)
	}
	// Only an exact name: yaml is PyYAML's module and psycopg2 is
	// psycopg2-binary's, which no table maps here.
	for _, f := range got[1:] {
		if f.Hint != "" {
			t.Errorf("unexpected hint %q for %q", f.Hint, f.Message)
		}
	}
}

func TestRuntimeHintSkipsAPackageTheProjectLists(t *testing.T) {
	d, _ := testDeps(t)
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return constraintsCatalog(t) }
	d.RuntimeFreeze = func(context.Context, string, string) ([]string, error) { return []string{"pandas==2.3.3"}, nil }
	c := &cli{d: d}
	m, err := manifest.Parse([]byte("[project]\nname = 'demo'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*', 'pandas']\n\n[tool.astro]\n"))
	if err != nil {
		t.Fatal(err)
	}
	res := checks.Result{Findings: []checks.Finding{{Kind: checks.KindImportError, Message: "ModuleNotFoundError: No module named 'pandas'"}}}
	if got := c.withRuntimeHints(context.Background(), t.TempDir(), m, res).Findings[0].Hint; got != "" {
		t.Errorf("a listed package got a hint: %q", got)
	}
}
