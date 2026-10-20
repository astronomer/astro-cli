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
		"apache-airflow-providers-common-sql", "apache-airflow-providers-common-io",
		"apache-airflow-providers-cncf-kubernetes",
		"opentelemetry-sdk", "kubernetes-asyncio", "openlineage-python", "googleapis-common-protos",
		// Shipped distributions whose names start with a module's they do not
		// provide.
		"markdown-it-py", "prometheus-client", "cron-descriptor", "google-cloud-storage",
	} {
		testShips[d] = true
	}
}

func TestRuntimeDist(t *testing.T) {
	for module, want := range map[string]string{
		"pandas":                            "pandas",
		"yaml":                              "pyyaml",
		"google.protobuf":                   "protobuf",
		"google.rpc":                        "googleapis-common-protos",
		"psycopg2":                          "psycopg2-binary",
		"MySQLdb":                           "mysqlclient",
		"airflow.providers.common.sql":      "apache-airflow-providers-common-sql",
		"airflow.providers.cncf.kubernetes": "apache-airflow-providers-cncf-kubernetes",
		"opentelemetry.sdk":                 "opentelemetry-sdk",
		"kubernetes_asyncio":                "kubernetes-asyncio",
		"openlineage.client":                "openlineage-python",
		// Python names the first missing package, so a provider namespace is
		// what a Dag sees when no provider under it is installed: the one
		// shipped under it is the answer.
		"airflow.providers.cncf": "apache-airflow-providers-cncf-kubernetes",
		// Several under it, and the message cannot say which one.
		"airflow.providers.common": "",
		// No other namespace is read that way: a shipped name that starts
		// with the module's is not evidence it provides it.
		"markdown":     "",
		"prometheus":   "",
		"cron":         "",
		"openlineage":  "",
		"google":       "",
		"google.cloud": "",
		// A missing submodule means its parent is installed: the parent's
		// distribution is not what is missing.
		"pandas.io.foo":                          "",
		"google.protobuf.message":                "",
		"airflow.providers.common.sql.hooks.sql": "",
		"airflow.providers.snowflake":            "",
		"my_company_utils":                       "",
		"":                                       "",
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
	// A chained traceback: the error raised is the last one named.
	chained := "ModuleNotFoundError: No module named 'ujson'\n\nDuring handling of the above exception, another exception occurred:\n\n" +
		"Traceback (most recent call last):\nModuleNotFoundError: No module named 'pandas'\n"
	if got := missingModule(chained); got != "pandas" {
		t.Errorf("missingModule(chained) = %q, want pandas", got)
	}
	// One that ends in another exception did not fail for the module.
	endsElsewhere := "Traceback (most recent call last):\nModuleNotFoundError: No module named 'ujson'\n\nDuring handling of the above exception, another exception occurred:\n\n" +
		"Traceback (most recent call last):\n  File \"dags/a.py\", line 4, in <module>\n    json = ujson\nNameError: name 'ujson' is not defined\n"
	if got := missingModule(endsElsewhere); got != "" {
		t.Errorf("missingModule(ends in NameError) = %q, want none", got)
	}
	if got := missingModule("ImportError: No module named 'yaml'"); got != "yaml" {
		t.Errorf("missingModule(ImportError) = %q, want yaml", got)
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

// A project that lists psycopg2 has the module psycopg2, which the image ships
// as psycopg2-binary: the same module, so no hint for the other distribution.
func TestCheckDoesNotHintAModuleADeclaredDependencyProvides(t *testing.T) {
	d, out, _, _ := hintDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	manifestText := strings.Replace(runtimeManifest, `"pandas"`, `"psycopg2"`, 1)
	if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestText), 0o600); err != nil {
		t.Fatal(err)
	}
	d.Checks = stubParser{report: checks.ParseReport{ImportErrors: []checks.ReportImportErr{
		{File: "dags/a.py", Message: "ModuleNotFoundError: No module named 'psycopg2'"},
	}}}
	_ = execute(t, d, "local", "check")
	if strings.Contains(out.String(), "hint:") {
		t.Errorf("want no hint for a module a declared dependency provides:\n%s", out.String())
	}
}

// An extra or a dependency group is listed too: the package is the project's,
// and its environment is the one that is short of it.
func TestCheckDoesNotHintAPackageAnExtraOrGroupLists(t *testing.T) {
	for name, section := range map[string]string{
		"extra": "[project.optional-dependencies]\nscience = [\"numpy>=2\"]\n",
		"group": "[dependency-groups]\ndev = [{include-group = \"lint\"}, \"numpy\"]\nlint = [\"ruff\"]\n",
	} {
		t.Run(name, func(t *testing.T) {
			d, out, _, _ := hintDeps(t)
			dir, err := d.WorkingDir()
			if err != nil {
				t.Fatal(err)
			}
			manifestText := strings.Replace(runtimeManifest, "[tool.astro]", section+"[tool.astro]", 1)
			if err := os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifestText), 0o600); err != nil {
				t.Fatal(err)
			}
			d.Checks = stubParser{report: numpyErrors()}
			_ = execute(t, d, "local", "check")
			if strings.Contains(out.String(), "hint:") {
				t.Errorf("want no hint for a package the %s lists:\n%s", name, out.String())
			}
		})
	}
}

// A package the lock already brings in (a dependency of a dependency) is
// missing from a stale environment, not from the manifest: no hint.
func TestCheckDoesNotHintAPackageTheLockHas(t *testing.T) {
	d, out, _, _ := hintDeps(t)
	dir, err := d.WorkingDir()
	if err != nil {
		t.Fatal(err)
	}
	lock := "version = 1\n\n[[package]]\nname = \"pandas\"\nversion = \"2.3.0\"\n\n[[package]]\nname = \"numpy\"\nversion = \"2.3.1\"\n"
	if err := os.WriteFile(filepath.Join(dir, "uv.lock"), []byte(lock), 0o600); err != nil {
		t.Fatal(err)
	}
	d.Checks = stubParser{report: numpyErrors()}
	_ = execute(t, d, "local", "check")
	if strings.Contains(out.String(), "hint:") {
		t.Errorf("want no hint for a package in uv.lock:\n%s", out.String())
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
