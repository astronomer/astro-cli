package local

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

const constraints339 = "SQLAlchemy==2.0.50"

func constraintsCatalog(t *testing.T) *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersionsV3": {
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.12", "3.13", "3.14"], "defaultPythonVersion": "3.14"}}
  }}`))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// startWithConstraints runs `astro local start` against a project whose
// manifest is src, the runtime's constraints file answering lines and err, and
// returns the manifest the start left, its output, and the builds and
// Pythons the file was asked for.
func startWithConstraints(t *testing.T, src string, lines []string, err error) (manifest, output string, asked []string) {
	t.Helper()
	t.Setenv("UV_EXCLUDE_NEWER", "")
	d, stdout := testDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")
	if werr := os.WriteFile(path, []byte(src), 0o600); werr != nil {
		t.Fatal(werr)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return constraintsCatalog(t) }
	d.RuntimeConstraints = func(_ context.Context, build, python string) ([]string, error) {
		asked = append(asked, build+"/"+python)
		return lines, err
	}
	_ = execute(t, d, "local", "start")
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return string(data), stdout.String() + stderr.String(), asked
}

var runtimeFile = []string{"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1", constraints339}

func TestStartWritesWhatTheRuntimeConstrains(t *testing.T) {
	got, out, asked := startWithConstraints(t, buildManifest, runtimeFile, nil)
	if len(asked) != 1 || asked[0] != "3.3-9/3.14" {
		t.Errorf("the constraints were asked for %q, want 3.3-9 on the project's Python 3.14", asked)
	}
	record := "# Astro Runtime 3.3-9 (Python 3.14) constrains these. astro writes them."
	ri, si, pi := strings.Index(got, record), strings.Index(got, "'"+constraints339+"'"), strings.Index(got, "apache-airflow-task-sdk==1.3.2+astro.1")
	if ri < 0 || si < 0 || pi < 0 || !(pi < ri && ri < si) {
		t.Errorf("want the +astro pins, then the record, then SQLAlchemy:\n%s", got)
	}
	if strings.Contains(got, "apache-airflow==3.3.2+astro.2") || strings.Contains(got, "task-sdk==1.3.2+astro.1',\n    'apache-airflow-task-sdk") {
		t.Errorf("the runtime's own Airflow line was written beside the build pin:\n%s", got)
	}
	if !strings.Contains(out, "now holds what Astro Runtime 3.3-9 constrains") {
		t.Errorf("the start did not say it recorded the runtime:\n%s", out)
	}

	again, out, _ := startWithConstraints(t, got, runtimeFile, nil)
	if again != got || strings.Contains(out, "now holds") {
		t.Errorf("a second start with the same runtime changed the manifest:\n%s", again)
	}
}

func TestStartKeepsTheProjectsOwnConstraints(t *testing.T) {
	src := strings.Replace(buildManifest, "[tool.astro]\n", "[tool.astro]\n\n[tool.uv]\nconstraint-dependencies = [\n    'urllib3<3',\n]\n", 1)
	got, _, _ := startWithConstraints(t, src, runtimeFile, nil)
	if !strings.Contains(got, "'urllib3<3'") || !strings.Contains(got, constraints339) {
		t.Errorf("want the project's constraint kept beside the runtime's:\n%s", got)
	}
}

func TestStartLeavesTheRuntimesLinesWhenTheyCannotBeRead(t *testing.T) {
	first, _, _ := startWithConstraints(t, buildManifest, runtimeFile, nil)

	offline, _, _ := startWithConstraints(t, first, nil, errors.New("dial tcp: no route to host"))
	if offline != first {
		t.Errorf("a file that cannot be read changed the manifest:\n%s", offline)
	}

	gone, _, _ := startWithConstraints(t, first, nil, runtimecatalog.ErrNotPublished)
	if strings.Contains(gone, "constrains these") || strings.Contains(gone, constraints339) {
		t.Errorf("a build with no constraints file kept the record:\n%s", gone)
	}
	if !strings.Contains(gone, "apache-airflow==3.3.2+astro.1") {
		t.Errorf("taking out the runtime's lines took the build pins with them:\n%s", gone)
	}
}

// init in an existing project that writes the runtime's lines reports
// pyproject.toml as updated, not up to date, and says the lock is behind.
func TestInitInAnExistingProjectReportsTheRuntimeLines(t *testing.T) {
	t.Setenv("UV_EXCLUDE_NEWER", "")
	d, stdout := testDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	dir := t.TempDir()
	for name, body := range map[string]string{"pyproject.toml": buildManifest, "uv.lock": "version = 1\n"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return constraintsCatalog(t) }
	d.RuntimeConstraints = func(context.Context, string, string) ([]string, error) { return runtimeFile, nil }

	if err := execute(t, d, "init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if strings.Contains(stdout.String(), "is up to date") {
		t.Errorf("init called a project it changed up to date:\n%s", stdout)
	}
	if !strings.Contains(stderr.String(), "uv.lock: run uv lock so it follows pyproject.toml") {
		t.Errorf("init did not say the lock is behind:\n%s", stderr)
	}
}

func TestStartUnderACutoffWritesNoRuntimeLines(t *testing.T) {
	src := strings.Replace(buildManifest, "[tool.astro]\n", "[tool.astro]\n\n[tool.uv]\nexclude-newer = '2026-01-01'\n", 1)
	got, _, asked := startWithConstraints(t, src, runtimeFile, nil)
	if len(asked) != 0 || strings.Contains(got, constraints339) {
		t.Errorf("a cutoff read or wrote the runtime's constraints (asked %q):\n%s", asked, got)
	}
}
