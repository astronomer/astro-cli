package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

const buildManifest = "[project]\nname = 'demo'\nrequires-python = '>=3.12'\ndependencies = ['apache-airflow==3.3.*']\n\n[tool.astro]\n"

var build332 = runtimeversions.AstroBuild{Index: "https://pip.astronomer.io/v2/", Airflow: "3.3.2+astro.1", TaskSDK: "1.3.2+astro.1"}

// startWithBuild runs `astro local start` against a project whose manifest is
// src, with the build lookup answering b and err, and returns the manifest the
// start left and what it wrote to stdout and stderr.
func startWithBuild(t *testing.T, src string, b runtimeversions.AstroBuild, err error) (manifest, output string) {
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
	var asked []string
	d.AstroBuild = func(_ context.Context, pin, runtime string) (runtimeversions.AstroBuild, error) {
		asked = append(asked, pin, runtime)
		return b, err
	}
	// The fake runtime refuses to start; what is asserted happens before it.
	_ = execute(t, d, "local", "start")
	if len(asked) != 2 || asked[0] != "3.3" || asked[1] != "" {
		t.Errorf("lookup asked with %q, want the manifest's pin and no runtime build", asked)
	}
	data, rerr := os.ReadFile(path)
	if rerr != nil {
		t.Fatal(rerr)
	}
	return string(data), stdout.String() + stderr.String()
}

func TestStartWritesTheAstroBuildIntoTheManifest(t *testing.T) {
	got, out := startWithBuild(t, buildManifest, build332, nil)

	for _, want := range []string{
		"'apache-airflow-core'",
		"apache-airflow==3.3.2+astro.1",
		"apache-airflow-task-sdk==1.3.2+astro.1",
		"[[tool.uv.index]]",
		"[tool.uv.sources]",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("manifest lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(out, "now installs Astronomer's Airflow 3.3.2+astro.1") {
		t.Errorf("the start did not say it changed the manifest:\n%s", out)
	}

	again, out := startWithBuild(t, got, build332, nil)
	if again != got || strings.Contains(out, "now installs") {
		t.Errorf("a second start with the same build changed the manifest:\n%s", out)
	}
}

// A pin moved while the index cannot be read leaves a build pin the new
// requirement excludes, which uv cannot satisfy, so it is taken out.
func TestStartTakesOutABuildPinThePinNoLongerCovers(t *testing.T) {
	written, _ := startWithBuild(t, buildManifest, build332, nil)
	moved := strings.Replace(written, "apache-airflow==3.3.*", "apache-airflow==3.2.*", 1)

	d, _ := testDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")
	if err := os.WriteFile(path, []byte(moved), 0o600); err != nil {
		t.Fatal(err)
	}
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) {
		return runtimeversions.AstroBuild{}, errors.New("dial tcp: no route to host")
	}
	_ = execute(t, d, "local", "start")

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "+astro.") {
		t.Errorf("a pin to a build of 3.3 stayed beside the requirement 3.2.*:\n%s", got)
	}
}

func TestInitWritesTheAstroBuild(t *testing.T) {
	d, stdout := testDeps(t)
	dir := t.TempDir()
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }

	if err := execute(t, d, "init", "--name", "demo", "--airflow-version", "3.3"); err != nil {
		t.Fatalf("init: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "apache-airflow==3.3.2+astro.1") || !strings.Contains(string(data), "[[tool.uv.index]]") {
		t.Errorf("init wrote no Astronomer build:\n%s", data)
	}
	if strings.Contains(stdout.String(), "warning") {
		t.Errorf("init printed a warning:\n%s", stdout)
	}
}

// An index that cannot be read says nothing about whether a build exists, so
// the manifest is left as it stands; one that has no build takes the settings
// out, and Airflow comes from PyPI.
func TestStartLeavesTheManifestAloneWhenTheIndexCannotBeRead(t *testing.T) {
	written, _ := startWithBuild(t, buildManifest, build332, nil)

	got, out := startWithBuild(t, written, runtimeversions.AstroBuild{}, errors.New("dial tcp: no route to host"))
	if got != written {
		t.Errorf("an unreadable index changed the manifest:\n%s", got)
	}
	if !strings.Contains(out, "used as it stands") {
		t.Errorf("the start did not say it could not check:\n%s", out)
	}

	got, out = startWithBuild(t, written, runtimeversions.AstroBuild{}, runtimeversions.ErrNoAstroBuild)
	if strings.Contains(got, "+astro.") || strings.Contains(got, "[tool.uv.sources]") {
		t.Errorf("no build on the index, but the manifest still points at one:\n%s", got)
	}
	if !strings.Contains(out, "has no build of it") || strings.Contains(out, "could not be read") {
		t.Errorf("the start did not say the index has no build:\n%s", out)
	}
}

// A new project's manifest is created, and the build init writes into it is
// part of that: it is listed once, as created, never again as updated.
func TestInitListsTheNewManifestOnce(t *testing.T) {
	d, stdout := testDeps(t)
	dir := t.TempDir()
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(context.Context, string, string) (runtimeversions.AstroBuild, error) { return build332, nil }

	if err := execute(t, d, "init", "--name", "demo", "--airflow-version", "3.3", "-o", "json"); err != nil {
		t.Fatalf("init: %v", err)
	}
	var res struct {
		Created []string `json:"created"`
		Updated []string `json:"updated"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("init -o json: %v\n%s", err, stdout)
	}
	if !slices.Contains(res.Created, "pyproject.toml") || slices.Contains(res.Updated, "pyproject.toml") {
		t.Errorf("created %v, updated %v: want pyproject.toml created only", res.Created, res.Updated)
	}
}

func TestListsFile(t *testing.T) {
	for entries, want := range map[string]bool{
		"pyproject.toml":                        true,
		"pyproject.toml (added [tool.astro])":   true,
		"pyproject.toml.bak":                    false,
		"dags/pyproject.toml (added something)": false,
	} {
		if got := listsFile([]string{entries}, "pyproject.toml"); got != want {
			t.Errorf("listsFile(%q) = %v, want %v", entries, got, want)
		}
	}
}
