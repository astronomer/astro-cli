package local

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/uv"
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

// seedRecord339 is the record a seed from freeze339 leaves in the manifest.
const seedRecord339 = "# astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12)."

// fakeUV stands in for `uv lock`. With the seed's constraints in the
// project's pyproject.toml it writes a lock holding every one at its version,
// as a resolution held to them would; without them it keeps the lock there,
// as uv's preferences keep a seeded lock's versions, and says so by leaving
// it alone.
type fakeUV struct {
	// seeded are the constraints the last seed lock stated.
	seeded []string
	// seedLocks and projectLocks count the two kinds of lock.
	seedLocks, projectLocks int
	// err fails every lock; projectErr fails only the project's.
	err, projectErr error
	// requiredOnly locks as uv does, a constraint adding no package: only
	// what [project] dependencies or a dependency group names is written.
	requiredOnly bool
}

func (f *fakeUV) lock(_ context.Context, dir string) error {
	if f.err != nil {
		return f.err
	}
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	if err != nil {
		return err
	}
	m, err := manifest.Parse(data)
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(m.UV.ConstraintDependencies, func(c string) bool { return strings.Contains(c, "python_version") }) {
		f.projectLocks++
		return f.projectErr
	}
	f.seedLocks++
	f.seeded = nil
	var lock strings.Builder
	lock.WriteString("version = 1\n")
	for _, c := range m.UV.ConstraintDependencies {
		if !strings.Contains(c, "python_version") {
			continue
		}
		f.seeded = append(f.seeded, c)
		head, _, _ := strings.Cut(c, ";")
		name, version, _ := strings.Cut(head, "==")
		if f.requiredOnly && !requires(m, manifest.DistName(name)) {
			continue
		}
		fmt.Fprintf(&lock, "\n[[package]]\nname = %q\nversion = %q\n", manifest.DistName(name), strings.TrimSpace(version))
	}
	return os.WriteFile(filepath.Join(dir, "uv.lock"), []byte(lock.String()), 0o600)
}

// pinsRun is one `astro local start` against src, with files written beside
// it, the build lookup answering build and lookupErr, the catalog listing
// pinsCatalog, the package list answering freeze and freezeErr, and uv.
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
	// uv locks, when set; a fresh fakeUV otherwise.
	uv *fakeUV
	// init runs the write as `astro init` does, seeding whatever the lock
	// holds, in place of a start.
	init bool
	// roots and rootsErr answer the runtime group's list; with neither set
	// there is no list to ask (Deps.RuntimeRoots nil), and the group is left
	// alone.
	roots    []string
	leftOut  []runtimecatalog.LeftOut
	rootsErr error
}

// pinsResult is what a pinsRun left: the manifest and the lock, what it
// printed, and what the build lookup and the package list were asked.
type pinsResult struct {
	manifest, lock, out                  string
	lookupAsked, freezeAsked, rootsAsked []string
	catalogReads                         int
	uv                                   *fakeUV
	// rootsFreeze is the package list the roots were asked with.
	rootsFreeze []string
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
	res := pinsResult{uv: p.uv}
	if res.uv == nil {
		res.uv = &fakeUV{}
	}
	d.Lock = res.uv.lock
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
		res.catalogReads++
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
	if p.roots != nil || p.rootsErr != nil {
		d.RuntimeRoots = func(_ context.Context, build, python string, freeze []string) (runtimecatalog.Roots, error) {
			res.rootsAsked = append(res.rootsAsked, build, python)
			res.rootsFreeze = freeze
			return runtimecatalog.Roots{Names: p.roots, LeftOut: p.leftOut}, p.rootsErr
		}
	}
	if p.init {
		(&cli{d: d}).writeAstroBuild(context.Background(), dir, true)
	} else {
		_ = execute(t, d, "local", "start")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lock, _ := os.ReadFile(filepath.Join(dir, "uv.lock"))
	res.manifest, res.lock, res.out = string(data), string(lock), stdout.String()+stderr.String()
	return res
}

// seeded is a start of src with no lock, answering freeze339: the manifest
// and lock it leaves, for a test to start from.
func seeded(t *testing.T) pinsResult {
	t.Helper()
	return pinsRun{src: buildManifest, build: build332, freeze: freeze339}.start(t)
}

func TestStartSeedsTheLock(t *testing.T) {
	r := seeded(t)
	if strings.Join(r.freezeAsked, " ") != "3.3-9 3.12" {
		t.Errorf("package list asked for %q, want the series' newest build and its default Python", r.freezeAsked)
	}
	want := []string{"pandas==2.2.3; python_version == '3.12'", "SQLAlchemy==2.0.50; python_version == '3.12'"}
	if strings.Join(r.uv.seeded, "|") != strings.Join(want, "|") {
		t.Errorf("seeded with %q, want %q", r.uv.seeded, want)
	}
	if r.uv.seedLocks != 1 || r.uv.projectLocks != 1 {
		t.Errorf("locked %d seed locks and the project %d times, want 1 and 1", r.uv.seedLocks, r.uv.projectLocks)
	}
	if !strings.Contains(r.lock, `name = "pandas"`) || !strings.Contains(r.lock, `version = "2.2.3"`) {
		t.Errorf("the project's lock does not hold the seeded versions:\n%s", r.lock)
	}
	for _, want := range []string{
		seedRecord339,
		// The image's own build of Airflow, not the newest on the index.
		"'apache-airflow==3.3.2+astro.2'",
	} {
		if !strings.Contains(r.manifest, want) {
			t.Errorf("manifest lacks %q:\n%s", want, r.manifest)
		}
	}
	if strings.Contains(r.manifest, "pandas") || strings.Contains(r.manifest, "python_version") {
		t.Errorf("the image's versions were written into the manifest:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "uv.lock: seeded with the versions Astro Runtime 3.3-9 (Python 3.12) ships (2 packages)") {
		t.Errorf("the start did not say it seeded the lock:\n%s", r.out)
	}

	// With the lock there, a start seeds nothing, and the lock is the image's.
	again := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339}.start(t)
	if again.manifest != r.manifest || again.uv.seedLocks != 0 || strings.Contains(again.out, "uv.lock:") || strings.Contains(again.out, "now installs") {
		t.Errorf("a second start changed something (seed locks %d):\n%s", again.uv.seedLocks, again.out)
	}

	// A package the project gives a bound the image's version does not meet
	// is its own to version, and the seed leaves it out.
	listed := strings.Replace(buildManifest, "'apache-airflow==3.3.*']", "'apache-airflow==3.3.*', 'pandas>=3']", 1)
	own := pinsRun{src: listed, build: build332, freeze: freeze339}.start(t)
	if strings.Contains(strings.Join(own.uv.seeded, " "), "pandas") {
		t.Errorf("the seed held pandas beside the project's own pandas>=3: %q", own.uv.seeded)
	}
}

// A lock that is there and holds other versions than the image is not
// seeded again by a start: the start says how many differ, less the ones the
// project chooses itself, and names the command that seeds it again.
func TestStartSaysHowFarTheLockIsFromTheImage(t *testing.T) {
	r := seeded(t)
	drifted := strings.Replace(strings.Replace(r.lock, `"2.2.3"`, `"3.0.1"`, 1), `"2.0.50"`, `"2.1.5"`, 1)
	got := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": drifted}, build: build332, freeze: freeze339}.start(t)
	if !strings.Contains(got.out, "uv.lock: 2 packages differ from Astro Runtime 3.3-9; run astro init to re-seed") {
		t.Errorf("no drift notice:\n%s", got.out)
	}
	if got.uv.seedLocks != 0 || got.lock != drifted {
		t.Error("a start seeded a lock that was there")
	}

	listed := strings.Replace(r.manifest, "'apache-airflow==3.3.*',", "'apache-airflow==3.3.*',\n    'pandas>=3',", 1)
	got = pinsRun{src: listed, files: map[string]string{"uv.lock": drifted}, build: build332, freeze: freeze339}.start(t)
	if !strings.Contains(got.out, "uv.lock: 1 packages differ") {
		t.Errorf("the project's own pandas counted as drift:\n%s", got.out)
	}
}

// init seeds the lock whatever it holds.
func TestInitSeedsALockThatIsThere(t *testing.T) {
	r := seeded(t)
	drifted := strings.Replace(r.lock, `"2.2.3"`, `"3.0.1"`, 1)
	got := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": drifted}, build: build332, freeze: freeze339, init: true}.start(t)
	if got.uv.seedLocks != 1 || !strings.Contains(got.lock, `"2.2.3"`) {
		t.Errorf("init did not seed the lock again (seed locks %d):\n%s", got.uv.seedLocks, got.lock)
	}
}

// A seed that fails leaves the lock and the manifest's record as they were,
// and says why: uv missing, or the project not locking with the versions.
func TestAFailedSeedLeavesTheLockAsItWas(t *testing.T) {
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, uv: &fakeUV{err: fmt.Errorf("uv: %w", uv.ErrNotFound)}}.start(t)
	if strings.Contains(r.manifest, "astro seeded") || r.lock != "" {
		t.Errorf("a seed without uv left a record or a lock:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "uv.lock is not seeded with what Astro Runtime 3.3-9 ships: uv is not installed") {
		t.Errorf("no word of the missing uv:\n%s", r.out)
	}

	const before = "version = 1\n"
	r = pinsRun{
		src: buildManifest, files: map[string]string{"uv.lock": before}, build: build332, freeze: freeze339, init: true,
		uv: &fakeUV{projectErr: errors.New("no solution found")},
	}.start(t)
	if r.lock != before || strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("a seed the project does not lock with left the lock changed (%q) or a record", r.lock)
	}
	if strings.Contains(r.manifest, "python_version") {
		t.Errorf("a failed seed left the image's versions in pyproject.toml:\n%s", r.manifest)
	}
	// A seed that fails at its first lock, the one with the image's versions,
	// leaves both files as they were too.
	r = pinsRun{
		src: buildManifest, files: map[string]string{"uv.lock": before}, build: build332, freeze: freeze339, init: true,
		uv: &fakeUV{err: errors.New("no solution found")},
	}.start(t)
	if r.lock != before || strings.Contains(r.manifest, "python_version") || strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("a seed that did not resolve left something behind (%q):\n%s", r.lock, r.manifest)
	}
}

func TestStartKeepsTheSeedWhenTheListCannotBeRead(t *testing.T) {
	r := seeded(t)
	got := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freezeErr: errors.New("dial tcp: no route to host")}.start(t)
	if got.manifest != r.manifest {
		t.Errorf("an unreadable list changed the record:\n%s", got.manifest)
	}
	if strings.Count(got.out, "warning:") != 1 || !strings.Contains(got.out, "uv.lock stays seeded from Astro Runtime 3.3-9: could not read what Astro Runtime 3.3-9 ships") {
		t.Errorf("the start did not say, once, that it could not read the list:\n%s", got.out)
	}
	// A newer build whose list cannot be read keeps the seed too, while the
	// requirement still covers its Airflow.
	newer := pinsRun{
		src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332,
		catalog:   strings.Replace(pinsCatalog, `"3.3-9":`, `"3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}, "3.3-9":`, 1),
		freezeErr: errors.New("context deadline exceeded"),
	}.start(t)
	if newer.manifest != r.manifest || newer.lock != r.lock {
		t.Errorf("a newer build's unreadable list changed the record or the lock:\n%s", newer.manifest)
	}
}

// A start that cannot reach Astronomer's index keeps the seed as it is, even
// for a project whose Airflow comes from PyPI, which has no Astronomer build
// to keep.
func TestStartOfflineKeepsTheSeedOfAPyPIAirflow(t *testing.T) {
	pypi := pinsRun{src: buildManifest, build: runtimeversions.AstroBuild{}, lookupErr: runtimeversions.ErrNoAstroBuild, freeze: freeze339, init: true}.start(t)
	if !strings.Contains(pypi.manifest, seedRecord339) {
		t.Fatalf("no seed to start from:\n%s\n%s", pypi.manifest, pypi.out)
	}
	got := pinsRun{src: pypi.manifest, files: map[string]string{"uv.lock": pypi.lock}, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if got.manifest != pypi.manifest || got.lock != pypi.lock {
		t.Errorf("an offline start changed a PyPI Airflow's seed:\n%s", got.manifest)
	}
}

func TestStartTakesOutTheRecordOfABuildWithNoList(t *testing.T) {
	r := seeded(t)
	got := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freezeErr: runtimecatalog.ErrFreezeNotPublished}.start(t)
	if strings.Contains(got.manifest, "astro seeded") {
		t.Errorf("the record stayed for a build that publishes no list:\n%s", got.manifest)
	}
	if !strings.Contains(got.out, "no longer records an Astro Runtime uv.lock was seeded from") {
		t.Errorf("the start did not say it took the record out:\n%s", got.out)
	}
}

// A project with its own exclude-newer, in [tool.uv] or its uv.toml, reads no
// package list: versions newer than its cutoff would leave uv nothing to
// lock, so nothing is seeded and an earlier seed's record comes out. A
// UV_EXCLUDE_NEWER in the environment says nothing of the project: nothing is
// read or seeded either, and the record stays.
func TestStartSeedsNothingUnderACutoff(t *testing.T) {
	r := seeded(t)
	src := strings.Replace(r.manifest, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	got := pinsRun{src: src, build: build332, freeze: freeze339}.start(t)
	if len(got.freezeAsked) != 0 || strings.Contains(got.manifest, "astro seeded") || got.uv.seedLocks != 0 {
		t.Errorf("the list was read (%q), or a seed made or kept, under the project's cutoff:\n%s", got.freezeAsked, got.manifest)
	}
	// Nothing is resolved for a seed there will not be. (The start's plan
	// reads the catalog for the venv's Python on its own, after this.)
	cutoffReads := got.catalogReads
	plain := pinsRun{src: r.manifest, build: build332, freeze: freeze339}.start(t)
	if cutoffReads >= plain.catalogReads {
		t.Errorf("catalog read %d times under a cutoff, %d without: the cutoff should spare the seed's read", cutoffReads, plain.catalogReads)
	}

	got = pinsRun{src: r.manifest, files: map[string]string{"uv.toml": "exclude-newer = \"2026-01-01\"\n"}, build: build332, freeze: freeze339}.start(t)
	if len(got.freezeAsked) != 0 || strings.Contains(got.manifest, "astro seeded") {
		t.Errorf("the list was read (%q), or the record kept, under uv.toml's cutoff:\n%s", got.freezeAsked, got.manifest)
	}

	t.Setenv("UV_EXCLUDE_NEWER", "1 week")
	got = pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339}.start(t)
	if len(got.freezeAsked) != 0 || got.manifest != r.manifest || got.uv.seedLocks != 0 {
		t.Errorf("the list was read (%q), or the record changed, under UV_EXCLUDE_NEWER:\n%s", got.freezeAsked, got.manifest)
	}
}

// The pin moved while the index cannot be read: the Airflow build pins come
// out, as they did, and the old release's record with them.
func TestStartTakesOutTheRecordOfAMovedPin(t *testing.T) {
	r := seeded(t)
	moved := strings.Replace(r.manifest, "apache-airflow==3.3.*", "apache-airflow==3.2.*", 1)
	got := pinsRun{src: moved, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if strings.Contains(got.manifest, "+astro.") || strings.Contains(got.manifest, "astro seeded") {
		t.Errorf("the old release's pins or record stayed beside the moved pin:\n%s", got.manifest)
	}
}

// The catalog cannot be read, so which build carries the pin cannot be told.
// The record stays while its Airflow is one the pin covers, and comes out
// once the pin has moved off it.
func TestStartWithoutTheCatalogKeepsOnlyARecordThePinStillCovers(t *testing.T) {
	r := seeded(t)
	kept := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339, noCatalog: true}.start(t)
	if !strings.Contains(kept.manifest, seedRecord339) {
		t.Errorf("the record came out with the pin unchanged:\n%s", kept.manifest)
	}

	build322 := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.2.2+astro.1", TaskSDK: "1.2.2+astro.1"}
	moved := strings.Replace(r.manifest, "apache-airflow==3.3.*", "apache-airflow==3.2.*", 1)
	got := pinsRun{src: moved, build: build322, freeze: freeze339, noCatalog: true}.start(t)
	if strings.Contains(got.manifest, "astro seeded") {
		t.Errorf("a record of Airflow 3.3.2 stayed beside apache-airflow==3.2.*:\n%s", got.manifest)
	}
	if !strings.Contains(got.out, "its Airflow 3.3.2 is one the Airflow requirement no longer covers") {
		t.Errorf("the start did not say why:\n%s", got.out)
	}
}

// The index cannot be read and nothing moved: the build stays as it is, and
// a project with no lock still gets one seeded, which needs no index.
func TestStartOfflineStillSeedsALockThatIsNotThere(t *testing.T) {
	r := seeded(t)
	got := pinsRun{src: r.manifest, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if got.uv.seedLocks != 1 || !strings.Contains(got.manifest, seedRecord339) || !strings.Contains(got.manifest, "apache-airflow==3.3.2+astro.2") {
		t.Errorf("offline, no seed or the build changed (seed locks %d):\n%s", got.uv.seedLocks, got.manifest)
	}
}

// The index cannot be read and the pin has not moved: a cutoff the project
// has just set takes the record out, and the Airflow build stays.
func TestStartOfflineStillAppliesACutoff(t *testing.T) {
	r := seeded(t)
	src := strings.Replace(r.manifest, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	got := pinsRun{src: src, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339}.start(t)
	if strings.Contains(got.manifest, "astro seeded") {
		t.Errorf("the record kept offline under the project's new cutoff:\n%s", got.manifest)
	}
	if !strings.Contains(got.manifest, "apache-airflow==3.3.2+astro.2") {
		t.Errorf("the Airflow build changed offline:\n%s", got.manifest)
	}
}

// The seed and the Airflow pin are of one release, or there is no seed:
// here the image's list carries 3.3.1 and the lookup 3.3.2.
func TestStartSeedsNothingOfAnotherRelease(t *testing.T) {
	other := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}
	r := pinsRun{src: buildManifest, build: build332, freeze: other}.start(t)
	if strings.Contains(r.manifest, "astro seeded") || r.uv.seedLocks != 0 {
		t.Errorf("seeded with another Airflow release's versions:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "carries Airflow 3.3.1, and the Airflow requirement holds it to 3.3.2") {
		t.Errorf("the start did not say why:\n%s", r.out)
	}
}

// A declared Dockerfile names the build, so the Airflow lookup is asked for
// the release that build carries, and the seed is of that build.
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
	if !strings.Contains(r.manifest, seedRecord339) {
		t.Errorf("no seed of the Dockerfile's build:\n%s", r.manifest)
	}
}

// patchCatalog lists a series whose newest build defaults to a Python its
// older build does not ship.
const patchCatalog = `{"runtimeVersionsV3": {
    "3.3-8": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}},
    "3.3-9": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "pythonVersions": ["3.12", "3.13"], "defaultPythonVersion": "3.13"}}
  }}`

// A patch pin's seed is of its build, on the Python the project runs, which
// the series' newest build decides (3.13 here): the venv and the image run
// it. When the build publishes no list for that Python there is no seed,
// since one of another Python holds versions of a Python the venv never
// runs, and the start says why.
func TestAPatchPinsSeedIsOfThePythonTheProjectRuns(t *testing.T) {
	build331 := runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.3.1+astro.1", TaskSDK: "1.3.1+astro.1"}
	src := strings.Replace(buildManifest, "apache-airflow==3.3.*", "apache-airflow==3.3.1", 1)
	list := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}

	r := pinsRun{src: src, build: build331, catalog: patchCatalog, freeze: list}.start(t)
	if strings.Join(r.freezeAsked, " ") != "3.3-8 3.13" || !strings.Contains(strings.Join(r.uv.seeded, " "), "python_version == '3.13'") {
		t.Errorf("asked %q, seeded %q; want 3.3-8 on 3.13, the Python the project runs", r.freezeAsked, r.uv.seeded)
	}

	r = pinsRun{src: src, build: build331, catalog: patchCatalog, freezeFor: func(_, python string) ([]string, error) {
		if python == "3.13" {
			return nil, runtimecatalog.ErrFreezeNotPublished
		}
		return list, nil
	}}.start(t)
	if strings.Contains(r.manifest, "astro seeded") || r.uv.seedLocks != 0 {
		t.Errorf("seeded with a Python the project does not run:\n%s", r.manifest)
	}
	if strings.Join(r.freezeAsked, " ") != "3.3-8 3.13" {
		t.Errorf("asked %q; want 3.3-8 on 3.13 alone, no second read", r.freezeAsked)
	}
	if !strings.Contains(r.out, "Astro Runtime 3.3-8 publishes no package list for Python 3.13, the one this project runs") {
		t.Errorf("the start did not say why:\n%s", r.out)
	}
}

// A write the manifest cannot be read back after says only that it changed
// the file, rather than which part it guessed.
func TestCompareWithoutTheManifestSaysOnlyItUpdated(t *testing.T) {
	var res astroBuildWrite
	res.compare(&manifest.Manifest{}, filepath.Join(t.TempDir(), "pyproject.toml"), true, false)
	if !res.updated || res.airflowChanged || res.recordChanged {
		t.Errorf("updated %v, airflowChanged %v, recordChanged %v; want updated alone", res.updated, res.airflowChanged, res.recordChanged)
	}
}

// Astronomer's index has no build of the pin, so Airflow comes from PyPI,
// but the runtime build carries the release the pin covers: it seeds the
// lock. It does not when the build carries another.
func TestStartSeedsWhenAirflowComesFromPyPI(t *testing.T) {
	r := pinsRun{src: buildManifest, lookupErr: runtimeversions.ErrNoAstroBuild, freeze: freeze339}.start(t)
	if !strings.Contains(r.manifest, seedRecord339) || strings.Contains(r.manifest, "+astro.") {
		t.Errorf("want the build's seed and no Astronomer Airflow:\n%s", r.manifest)
	}

	src := strings.Replace(buildManifest, "apache-airflow==3.3.*", "apache-airflow==3.3.5", 1)
	r = pinsRun{src: src, lookupErr: runtimeversions.ErrNoAstroBuild, freeze: freeze339}.start(t)
	if strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("a seed of Airflow 3.3.2 beside apache-airflow==3.3.5:\n%s", r.manifest)
	}
}

// A write that repairs only another setting that holds Airflow to the build,
// here exclude-newer-package, still says what it did.
func TestStartSaysSoWhenItRepairsTheBuildsSettings(t *testing.T) {
	r := seeded(t)
	start := strings.Index(r.manifest, "[tool.uv.exclude-newer-package]")
	if start < 0 {
		t.Fatalf("no exclude-newer-package to take out:\n%s", r.manifest)
	}
	got := pinsRun{src: r.manifest[:start], files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339}.start(t)
	if !strings.Contains(got.manifest, "[tool.uv.exclude-newer-package]") {
		t.Fatalf("not repaired:\n%s", got.manifest)
	}
	if !strings.Contains(got.out, "now installs Astronomer's Airflow") {
		t.Errorf("the repair was silent:\n%s", got.out)
	}
}

// The runtime pins an older astro wrote into the manifest come out at the
// next write, and the lock is seeded in their place.
func TestStartTakesOutTheRuntimePinsOfAnOlderAstro(t *testing.T) {
	src := buildManifest + "\n[tool.uv]\nconstraint-dependencies = [\n" +
		"    'apache-airflow==3.3.2+astro.2',\n" +
		"    # Astro Runtime 3.3-9 (Python 3.12) ships these versions, so the lockfile resolves to what the image runs.\n" +
		"    # astro writes them. To choose a version yourself, give the package a bound the image's version doesn't meet.\n" +
		"    \"pandas==2.2.3; python_version == '3.12'\",\n" +
		"    # End of what astro writes. Constraints below this line are yours.\n" +
		"    'numpy<3',\n]\n"
	r := pinsRun{src: src, build: build332, freeze: freeze339}.start(t)
	for _, gone := range []string{"ships these versions", "pandas==2.2.3", "End of what astro writes"} {
		if strings.Contains(r.manifest, gone) {
			t.Errorf("manifest still holds %q:\n%s", gone, r.manifest)
		}
	}
	if !strings.Contains(r.manifest, "'numpy<3'") || !strings.Contains(r.manifest, seedRecord339) {
		t.Errorf("the project's own constraint or the seed's record is missing:\n%s", r.manifest)
	}
}
