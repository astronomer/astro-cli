package local

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/checks"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// The image of these tests ships what a 3.3 runtime does, in part.
var testShips = map[string]bool{}

func init() {
	for _, d := range []string{
		"pandas", "numpy", "pyyaml", "protobuf", "psycopg2-binary", "mysqlclient",
		"apache-airflow-providers-common-sql", "apache-airflow-providers-cncf-kubernetes",
		"opentelemetry-sdk", "kubernetes-asyncio", "openlineage-python", "googleapis-common-protos",
	} {
		testShips[d] = true
	}
}

func TestRuntimeDist(t *testing.T) {
	for module, want := range map[string]string{
		"pandas":                                 "pandas",
		"pandas.core.frame":                      "pandas",
		"yaml":                                   "pyyaml",
		"google.protobuf.message":                "protobuf",
		"google.rpc":                             "googleapis-common-protos",
		"psycopg2":                               "psycopg2-binary",
		"MySQLdb":                                "mysqlclient",
		"airflow.providers.common.sql.hooks.sql": "apache-airflow-providers-common-sql",
		"airflow.providers.cncf.kubernetes":      "apache-airflow-providers-cncf-kubernetes",
		"opentelemetry.sdk.trace":                "opentelemetry-sdk",
		"kubernetes_asyncio":                     "kubernetes-asyncio",
		"openlineage.client.run":                 "openlineage-python",
		"airflow.providers.snowflake":            "",
		"my_company_utils":                       "",
		"google":                                 "",
	} {
		if got := runtimeDist(module, testShips); got != want {
			t.Errorf("runtimeDist(%q) = %q, want %q", module, got, want)
		}
	}
}

func TestMissingModule(t *testing.T) {
	msg := "Traceback (most recent call last):\n  File \"dags/a.py\", line 1, in <module>\n    import numpy\nModuleNotFoundError: No module named 'numpy'\n"
	if got := missingModule(msg); got != "numpy" {
		t.Errorf("missingModule = %q, want numpy", got)
	}
	if got := missingModule("ValueError: no"); got != "" {
		t.Errorf("missingModule = %q, want none", got)
	}
}

// hintDeps is checkDeps with a manifest naming its runtime build (runtimeManifest), so the
// build is known with no catalog, and a freeze that records what it was asked.
func hintDeps(t *testing.T) (d Deps, out *bytes.Buffer, asked *strings.Builder, pythons *[]string) {
	t.Helper()
	d, out = checkDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(runtimeManifest), 0o600); err != nil {
		t.Fatal(err)
	}
	asked, pythons = &strings.Builder{}, &[]string{}
	d.RuntimeFreeze = func(_ context.Context, build string, py []string) []string {
		asked.WriteString(build)
		*pythons = py
		var dists []string
		for dist := range testShips {
			dists = append(dists, dist)
		}
		return dists
	}
	return d, out, asked, pythons
}

const runtimeManifest = "[project]\nname = 'demo'\ndependencies = [\"apache-airflow==3.1.*\", \"pandas\"]\n[tool.astro]\nruntime = '3.1-12'\n"

func numpyErrors() checks.ParseReport {
	msg := "Traceback (most recent call last):\n  File \"x\", line 1\nModuleNotFoundError: No module named 'numpy'\n"
	return checks.ParseReport{ImportErrors: []checks.ReportImportErr{
		{File: "dags/a.py", Message: msg},
		{File: "dags/b.py", Message: msg},
	}}
}

// One missing package fails every Dag that imports it; the hint prints once.
func TestCheckHintsAPackageTheRuntimeShips(t *testing.T) {
	d, out, asked, pythons := hintDeps(t)
	d.Checks = stubParser{report: numpyErrors()}
	_ = execute(t, d, "local", "check")
	want := "hint: Astro Runtime 3.1-12 ships numpy, but this project does not depend on it: add it with uv add numpy"
	if n := strings.Count(out.String(), want); n != 1 {
		t.Errorf("want the hint once, got %d times:\n%s", n, out.String())
	}
	if asked.String() != "3.1-12" {
		t.Errorf("freeze asked for build %q, want 3.1-12", asked.String())
	}
	// No catalog: the fallbacks alone.
	if strings.Join(*pythons, ",") != strings.Join(fallbackPythons, ",") {
		t.Errorf("pythons = %v, want %v", *pythons, fallbackPythons)
	}
}

// --output json carries the hint on every finding it applies to.
func TestCheckJSONCarriesTheHintOnEachFinding(t *testing.T) {
	d, out, _, _ := hintDeps(t)
	d.Checks = stubParser{report: numpyErrors()}
	_ = execute(t, d, "local", "check", "--output", "json")
	hinted := 0
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		var f checks.Finding
		if json.Unmarshal([]byte(line), &f) == nil && f.Kind == checks.KindImportError && strings.Contains(f.Hint, "uv add numpy") {
			hinted++
		}
	}
	if hinted != 2 {
		t.Errorf("want 2 hinted findings, got %d:\n%s", hinted, out.String())
	}
}

// A package the project already lists is missing from a stale environment,
// which adding it again does not fix, so it gets no hint.
func TestCheckDoesNotHintAPackageTheProjectLists(t *testing.T) {
	d, out, _, _ := hintDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{ImportErrors: []checks.ReportImportErr{
		{File: "dags/a.py", Message: "ModuleNotFoundError: No module named 'pandas'"},
	}}}
	_ = execute(t, d, "local", "check")
	if strings.Contains(out.String(), "hint:") {
		t.Errorf("want no hint for a declared dependency:\n%s", out.String())
	}
}

// A check with no missing module asks nothing of the CDN.
func TestCheckReadsNoFreezeWithoutAMissingModule(t *testing.T) {
	d, _, asked, _ := hintDeps(t)
	d.Checks = stubParser{report: checks.ParseReport{ImportErrors: []checks.ReportImportErr{
		{File: "dags/a.py", Message: "ValueError: bad"},
	}}}
	_ = execute(t, d, "local", "check")
	if asked.Len() != 0 {
		t.Errorf("freeze read for build %q with no missing module", asked.String())
	}
}

// A declared Dockerfile names the build, and its tag's Python is tried first.
func TestRuntimeImageOfReadsADeclaredDockerfile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM quay.io/astronomer/astro-runtime:12.6.0-python-3.11\nRUN true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Project: manifest.Project{Dependencies: []string{"apache-airflow==2.10.*"}},
		Astro:   manifest.Astro{Dockerfile: "Dockerfile"},
	}
	c := &cli{d: Deps{}}
	build, pythons := c.runtimeImageOf(context.Background(), dir, m)
	if build != "12.6.0" {
		t.Errorf("build = %q, want 12.6.0", build)
	}
	if len(pythons) == 0 || pythons[0] != "3.11" {
		t.Errorf("pythons = %v, want 3.11 first", pythons)
	}
}

// With no build named and no catalog to find the newest, there is no image to
// ask about, and no hint.
func TestRuntimeImageOfNeedsABuild(t *testing.T) {
	m := &manifest.Manifest{Project: manifest.Project{Dependencies: []string{"apache-airflow==3.1.*"}}}
	c := &cli{d: Deps{}}
	if build, _ := c.runtimeImageOf(context.Background(), t.TempDir(), m); build != "" {
		t.Errorf("build = %q, want none", build)
	}
}
