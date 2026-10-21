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
	r := pinsRun{src: src, build: build332, freeze: freeze, freezeErr: err}.start(t)
	return r.manifest, r.out, r.freezeAsked
}

// pinsRun is one `astro local start` against src, with files written beside
// it, the build lookup answering build and lookupErr, the catalog listing
// pinsCatalog, and the package list answering freeze and freezeErr.
type pinsRun struct {
	src                  string
	files                map[string]string
	build                runtimeversions.AstroBuild
	lookupErr, freezeErr error
	freeze               []string
	// noCatalog has the runtime catalog unreadable, and catalog, when set,
	// is read in place of pinsCatalog.
	noCatalog bool
	catalog   string
	// freezeFor, when set, answers the package list by build and Python in
	// place of freeze and freezeErr.
	freezeFor func(build, python string) ([]string, error)
}

// pinsResult is what a pinsRun left: the manifest, what it printed, and what
// the build lookup and the package list were asked.
type pinsResult struct {
	manifest, out            string
	lookupAsked, freezeAsked []string
}

func (p pinsRun) start(t *testing.T) pinsResult {
	t.Helper()
	d, stdout := testDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	dir := t.TempDir()
	path := filepath.Join(dir, "pyproject.toml")
	if err := os.WriteFile(path, []byte(p.src), 0o600); err != nil {
		t.Fatal(err)
	}
	for name, body := range p.files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var res pinsResult
	d.WorkingDir = func() (string, error) { return dir, nil }
	d.AstroBuild = func(_ context.Context, pin, runtime string) (runtimeversions.AstroBuild, error) {
		res.lookupAsked = append(res.lookupAsked, pin, runtime)
		return p.build, p.lookupErr
	}
	catalog := pinsCatalog
	if p.catalog != "" {
		catalog = p.catalog
	}
	c, err := runtimeversions.Parse([]byte(catalog))
	if err != nil {
		t.Fatal(err)
	}
	d.RuntimeCatalog = func(context.Context) *runtimeversions.Catalog {
		if p.noCatalog {
			return nil
		}
		return c
	}
	d.RuntimePins = func(_ context.Context, build, python string) ([]string, error) {
		res.freezeAsked = append(res.freezeAsked, build, python)
		if p.freezeFor != nil {
			return p.freezeFor(build, python)
		}
		return p.freeze, p.freezeErr
	}
	_ = execute(t, d, "local", "start")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res.manifest, res.out = string(data), stdout.String()+stderr.String()
	return res
}

func TestStartWritesTheRuntimePins(t *testing.T) {
	got, out, asked := startWithPins(t, buildManifest, freeze339, nil)
	if strings.Join(asked, " ") != "3.3-9 3.12" {
		t.Errorf("package list asked for %q, want the series' newest build and its default Python", asked)
	}
	for _, want := range []string{
		"# Astro Runtime 3.3-9 (Python 3.12) ships these versions",
		"\"pandas==2.2.3; python_version == '3.12'\"",
		"\"SQLAlchemy==2.0.50; python_version == '3.12'\"",
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
	if strings.Contains(after, "pandas==2.2.3") {
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

// A project with its own exclude-newer reads no package list, as under
// UV_EXCLUDE_NEWER: pins newer than its cutoff would leave uv nothing to
// lock, so the ones there come out too.
func TestStartHoldsNoRuntimePinsUnderTheProjectsCutoff(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	src := strings.Replace(written, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	r := pinsRun{src: src, build: build332, freeze: freeze339}.start(t)
	if len(r.freezeAsked) != 0 || strings.Contains(r.manifest, "Astro Runtime") || strings.Contains(r.manifest, "pandas==") {
		t.Errorf("the list was read (%q), or its pins kept, under the project's cutoff:\n%s", r.freezeAsked, r.manifest)
	}
}

// The pin moved while the index cannot be read: the Airflow build pins come
// out, as they did, and the old release's runtime pins with them.
func TestStartTakesOutTheRuntimePinsOfAMovedPin(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	moved := strings.Replace(written, "apache-airflow==3.3.*", "apache-airflow==3.2.*", 1)
	r := pinsRun{src: moved, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if strings.Contains(r.manifest, "+astro.") || strings.Contains(r.manifest, "pandas==") || strings.Contains(r.manifest, "Astro Runtime") {
		t.Errorf("pins of the old release stayed beside the moved pin:\n%s", r.manifest)
	}
}

// The catalog cannot be read, so which build carries the pin cannot be told.
// The pins stay while their Airflow is one the pin covers, and come out once
// the pin has moved off it.
func TestStartWithoutTheCatalogKeepsOnlyPinsThePinStillCovers(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)

	kept := pinsRun{src: written, build: build332, freeze: freeze339, noCatalog: true}.start(t)
	if !strings.Contains(kept.manifest, "pandas==2.2.3") {
		t.Errorf("the pins came out with the pin unchanged:\n%s", kept.manifest)
	}

	build322 := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.2.2+astro.1", TaskSDK: "1.2.2+astro.1"}
	moved := strings.Replace(written, "apache-airflow==3.3.*", "apache-airflow==3.2.*", 1)
	r := pinsRun{src: moved, build: build322, freeze: freeze339, noCatalog: true}.start(t)
	if strings.Contains(r.manifest, "pandas==") || strings.Contains(r.manifest, "Astro Runtime") {
		t.Errorf("pins of Airflow 3.3.2 stayed beside apache-airflow==3.2.*:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "are of Airflow 3.3.2, which the Airflow requirement no longer covers") {
		t.Errorf("the start did not say why:\n%s", r.out)
	}
}

// The index cannot be read and nothing moved: the build stays as it is, but a
// package the project has since listed loses its pin, which needs no lookup.
func TestStartOfflineStillDropsThePinOfAPackageTheProjectLists(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	listed := strings.Replace(written, "'apache-airflow==3.3.*',", "'apache-airflow==3.3.*',\n    'pandas>=3',", 1)
	r := pinsRun{src: listed, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if strings.Contains(r.manifest, "pandas==") {
		t.Errorf("pandas kept its pin beside the project's own pandas>=3:\n%s", r.manifest)
	}
	if !strings.Contains(r.manifest, "SQLAlchemy==2.0.50") || !strings.Contains(r.manifest, "apache-airflow==3.3.2+astro.2") {
		t.Errorf("the rest of the build changed offline:\n%s", r.manifest)
	}
}

// The runtime pins and the Airflow pin are of one release, or none are
// written: here the image's list carries 3.3.1 and the lookup 3.3.2.
func TestStartWritesNoRuntimePinsOfAnotherRelease(t *testing.T) {
	other := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}
	r := pinsRun{src: buildManifest, build: build332, freeze: other}.start(t)
	if strings.Contains(r.manifest, "Astro Runtime") || strings.Contains(r.manifest, "pandas") {
		t.Errorf("pins of another Airflow release were written:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "carries Airflow 3.3.1, and the Airflow requirement holds it to 3.3.2") {
		t.Errorf("the start did not say why:\n%s", r.out)
	}
}

// A declared Dockerfile names the build, so the Airflow lookup is asked for
// the release that build carries.
func TestStartLooksUpTheAirflowOfTheDockerfilesBuild(t *testing.T) {
	src := strings.Replace(buildManifest, "[tool.astro]\n", "[tool.astro]\ndockerfile = 'Dockerfile'\n", 1)
	r := pinsRun{
		src:   src,
		files: map[string]string{"Dockerfile": "FROM astrocrpublic.azurecr.io/runtime:3.3-9\nRUN true\n"},
		build: build332, freeze: freeze339,
	}.start(t)
	if len(r.lookupAsked) != 2 || r.lookupAsked[1] != "3.3-9" {
		t.Errorf("lookup asked %q, want the Dockerfile's build", r.lookupAsked)
	}
	if !strings.Contains(r.manifest, "# Astro Runtime 3.3-9 (Python 3.12)") {
		t.Errorf("no pins of the Dockerfile's build:\n%s", r.manifest)
	}
}

// patchCatalog lists a series whose newest build defaults to a Python its
// older build does not ship.
const patchCatalog = `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}}
  }}`

// A patch pin's pins are of its build, on the Python the project runs, which
// the series' newest build decides (3.13 here): the venv and the image run
// it. When the build publishes no list for that Python, its own Python's
// list is used, with a warning.
func TestAPatchPinsPinsAreOfThePythonTheProjectRuns(t *testing.T) {
	build331 := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.3.1+astro.1", TaskSDK: "1.3.1+astro.1"}
	src := strings.Replace(buildManifest, "apache-airflow==3.3.*", "apache-airflow==3.3.1", 1)
	list := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}

	r := pinsRun{src: src, build: build331, catalog: patchCatalog, freeze: list}.start(t)
	if strings.Join(r.freezeAsked, " ") != "3.3-8 3.13" || !strings.Contains(r.manifest, "python_version == '3.13'") {
		t.Errorf("asked %q; want 3.3-8 on 3.13, the Python the project runs:\n%s", r.freezeAsked, r.manifest)
	}

	r = pinsRun{src: src, build: build331, catalog: patchCatalog, freezeFor: func(_, python string) ([]string, error) {
		if python == "3.13" {
			return nil, runtimecatalog.ErrFreezeNotPublished
		}
		return list, nil
	}}.start(t)
	if !strings.Contains(r.manifest, "# Astro Runtime 3.3-8 (Python 3.12)") {
		t.Errorf("no fallback to the build's own Python:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "publishes no package list for Python 3.13, the one this project runs") {
		t.Errorf("the start did not say it fell back:\n%s", r.out)
	}
}

// The index cannot be read and the pin has not moved: what needs no index
// still applies. A cutoff the project has just set takes the pins out.
func TestStartOfflineStillAppliesACutoff(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	src := strings.Replace(written, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	r := pinsRun{src: src, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if strings.Contains(r.manifest, "pandas==") || strings.Contains(r.manifest, "Astro Runtime") {
		t.Errorf("pins kept offline under the project's new cutoff:\n%s", r.manifest)
	}
	if !strings.Contains(r.manifest, "apache-airflow==3.3.2+astro.2") {
		t.Errorf("the Airflow build changed offline:\n%s", r.manifest)
	}
}

// The index cannot be read and the pin has not moved: a new requires-python
// that changes the Python the project runs is applied, from the cached list.
func TestStartOfflineStillAppliesANewPython(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	src := strings.Replace(written, "requires-python = '>=3.12'", "requires-python = '>=3.13'", 1)
	r := pinsRun{src: src, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if !strings.Contains(r.manifest, "# Astro Runtime 3.3-9 (Python 3.13)") {
		t.Errorf("the new Python was not applied offline (asked %q):\n%s", r.freezeAsked, r.manifest)
	}
}

// A write that repairs only another setting that holds Airflow to the build,
// here exclude-newer-package, still says what it did.
func TestStartSaysSoWhenItRepairsTheBuildsSettings(t *testing.T) {
	written, _, _ := startWithPins(t, buildManifest, freeze339, nil)
	start := strings.Index(written, "[tool.uv.exclude-newer-package]")
	if start < 0 {
		t.Fatalf("no exclude-newer-package to take out:\n%s", written)
	}
	r := pinsRun{src: written[:start], build: build332, freeze: freeze339}.start(t)
	if !strings.Contains(r.manifest, "[tool.uv.exclude-newer-package]") {
		t.Fatalf("not repaired:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "now installs Astronomer's Airflow") {
		t.Errorf("the repair was silent:\n%s", r.out)
	}
}
