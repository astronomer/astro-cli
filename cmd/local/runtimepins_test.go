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

const pinsCatalog = `{"runtimeVersionsV3": {
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.12"}}
  }}`

// freeze339 is the image's list: Airflow at a newer +astro build than the
// index lookup chose, which the image's own list overrides.
var freeze339 = []string{
	"apache-airflow==3.3.2+astro.2",
	"apache-airflow-task-sdk==1.3.2+astro.1",
	"pandas==2.2.3",
	"SQLAlchemy==2.0.50",
}

// startWithPins runs `astro local start` against src with the build lookup
// answering build332, the catalog listing 3.3-9, and the package list
// answering freeze and err. It returns the manifest the start left and what
// it printed, and the build and Python the list was asked for.
func startWithPins(t *testing.T, src string, freeze []string, err error) (got, out string, asked []string) {
	t.Helper()
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
	c, perr := runtimeversions.Parse([]byte(pinsCatalog))
	if perr != nil {
		t.Fatal(perr)
	}
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog { return c }
	d.RuntimePins = func(_ context.Context, build, python string) ([]string, error) {
		asked = append(asked, build, python)
		return freeze, err
	}
	_ = execute(t, d, "local", "start")
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return string(data), stdout.String() + stderr.String(), asked
}

func TestStartWritesTheRuntimePins(t *testing.T) {
	got, out, asked := startWithPins(t, buildManifest, freeze339, nil)
	if strings.Join(asked, " ") != "3.3-9 3.12" {
		t.Errorf("package list asked for %q, want the series' newest build and its default Python", asked)
	}
	for _, want := range []string{
		"# Astro Runtime 3.3-9 (Python 3.12) ships these versions",
		"'pandas==2.2.3'",
		"'SQLAlchemy==2.0.50'",
		// The image's own build of Airflow, not the newest on the index.
		"'apache-airflow==3.3.2+astro.2'",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(out, "now holds 2 packages to the versions Astro Runtime 3.3-9 (Python 3.12) ships") {
		t.Errorf("the start did not say it wrote the pins:\n%s", out)
	}

	again, out, _ := startWithPins(t, got, freeze339, nil)
	if again != got || strings.Contains(out, "now holds") || strings.Contains(out, "now installs") {
		t.Errorf("a second start with the same build changed the manifest:\n%s", out)
	}

	// A package the project lists is its own to version.
	listed := strings.Replace(got, "'apache-airflow==3.3.*',", "'apache-airflow==3.3.*',\n    'pandas>=3',", 1)
	after, _, _ := startWithPins(t, listed, freeze339, nil)
	if strings.Contains(after, "'pandas==2.2.3'") {
		t.Errorf("the pin stayed beside the project's own pandas:\n%s", after)
	}
}

func TestStartKeepsTheRuntimePinsWhenTheListCannotBeRead(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	got, out, _ := startWithPins(t, written, nil, errors.New("dial tcp: no route to host"))
	if got != written {
		t.Errorf("an unreadable list changed the pins:\n%s", got)
	}
	if !strings.Contains(out, "could not read what Astro Runtime 3.3-9 ships") {
		t.Errorf("the start did not say it could not read the list:\n%s", out)
	}
}

func TestStartTakesOutTheRuntimePinsOfABuildWithNoList(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	got, out, _ := startWithPins(t, written, nil, runtimecatalog.ErrFreezeNotPublished)
	if strings.Contains(got, "Astro Runtime") || strings.Contains(got, "pandas") {
		t.Errorf("the pins stayed for a build that publishes no list:\n%s", got)
	}
	if !strings.Contains(out, "no longer holds packages") {
		t.Errorf("the start did not say it took the pins out:\n%s", out)
	}
}
