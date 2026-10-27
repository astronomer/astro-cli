package local

import (
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// A seed that fails is not tried again by the next start while the project,
// its lock, the build and the Python are the same; init always tries.
func TestAFailedSeedIsNotTriedAgainOnEveryStart(t *testing.T) {
	cache, dir := t.TempDir(), t.TempDir()
	f := &fakeUV{err: errors.New("no solution found")}
	run := func(init bool) pinsResult {
		return pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: []string{"pandas"}, uv: f, cache: cache, dir: dir, init: init}.start(t)
	}
	first := run(false)
	if f.calls == 0 || !strings.Contains(first.out, "uv.lock is not seeded") {
		t.Fatalf("the first start did not try the seed (%d locks):\n%s", f.calls, first.out)
	}
	tried := f.calls
	again := run(false)
	if f.calls != tried {
		t.Errorf("the next start locked again (%d more) though nothing changed", f.calls-tried)
	}
	if again.manifest != first.manifest || again.lock != "" || strings.Contains(again.manifest, "astro seeded") {
		t.Errorf("a skipped seed changed the project:\n%s", again.manifest)
	}
	run(true)
	if f.calls == tried {
		t.Error("init did not try the seed again")
	}
}

// A seed whose lock holds none of the image's versions seeds nothing: no
// record and no group are written, the project is put back, and a start
// does not try it again.
func TestASeedThatHoldsNothingWritesNothing(t *testing.T) {
	cache, dir := t.TempDir(), t.TempDir()
	// The project requires none of the list's packages and the group lists
	// none, so uv, adding no package for a constraint, locks none of them.
	f := &fakeUV{requiredOnly: true}
	first := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: []string{"statsd"}, uv: f, cache: cache, dir: dir}.start(t)
	if strings.Contains(first.manifest, "astro seeded") || strings.Contains(first.manifest, "astro-runtime") {
		t.Errorf("a seed that held nothing wrote its record or its group:\n%s", first.manifest)
	}
	if first.lock != "" {
		t.Errorf("a seed that held nothing left a lock: %q", first.lock)
	}
	tried := f.calls
	pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: []string{"statsd"}, uv: f, cache: cache, dir: dir}.start(t)
	if f.calls != tried {
		t.Errorf("the next start tried the seed that held nothing again (%d more locks)", f.calls-tried)
	}
}

// A project whose dependency groups are an inline table cannot take the
// astro-runtime group. The seed still works, and the next start does not
// seed again for a group it can never write.
func TestARefusedGroupDoesNotReseedEveryStart(t *testing.T) {
	cache, dir := t.TempDir(), t.TempDir()
	src := "dependency-groups = { lint = ['ruff'] }\n" + buildManifest
	f := &fakeUV{}
	first := pinsRun{src: src, build: build332, freeze: freeze339, roots: []string{"pandas"}, uv: f, cache: cache, dir: dir}.start(t)
	if f.seedLocks != 1 || first.lock == "" {
		t.Fatalf("the first start did not seed (seed locks %d):\n%s", f.seedLocks, first.out)
	}
	pinsRun{src: first.manifest, files: map[string]string{"uv.lock": first.lock}, build: build332, freeze: freeze339, roots: []string{"pandas"}, uv: f, cache: cache, dir: dir}.start(t)
	if f.seedLocks != 1 {
		t.Errorf("a start re-seeded for a group it cannot write (seed locks %d)", f.seedLocks)
	}
}

// A newer build of the release the lock was seeded from does not re-seed on
// a start: the lock, its record and its group stay that build's, and the
// start says how far the lock is from the newer one.
func TestANewerBuildOfTheSameReleaseKeepsTheSeedOnAStart(t *testing.T) {
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: []string{"pandas"}}.start(t)
	if !strings.Contains(r.manifest, seedRecord339) {
		t.Fatalf("no seed to start from:\n%s\n%s", r.manifest, r.out)
	}
	newer := strings.Replace(pinsCatalog, `"3.3-9":`, `"3.3-10": {"metadata": {"airflowVersion": "3.3.2", "channel": "stable", "releaseDate": "2026-01-01", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}, "3.3-9":`, 1)
	f := &fakeUV{}
	got := pinsRun{
		src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, catalog: newer,
		freezeFor: func(build, _ string) ([]string, error) {
			if build == "3.3-10" {
				return []string{"apache-airflow==3.3.2+astro.2", "apache-airflow-task-sdk==1.3.2+astro.1", "pandas==2.3.0", "SQLAlchemy==2.0.50"}, nil
			}
			return freeze339, nil
		},
		roots: []string{"pandas"}, uv: f,
	}.start(t)
	if f.calls != 0 {
		t.Errorf("a start re-seeded for a newer build of the same release (%d locks)", f.calls)
	}
	if got.manifest != r.manifest {
		t.Errorf("the record or the group moved without a seed:\n%s", got.manifest)
	}
	if !strings.Contains(got.out, "differ from Astro Runtime 3.3-10; run astro init to re-seed") {
		t.Errorf("no word of the newer build:\n%s", got.out)
	}
}

// An Airflow pin that moved off the release of the build the lock was seeded
// from re-seeds on a start: the lock is another Airflow's.
func TestAPinThatMovesOffTheSeedsReleaseReseedsOnAStart(t *testing.T) {
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: []string{"pandas"}}.start(t)
	if !strings.Contains(r.manifest, seedRecord339) {
		t.Fatalf("no seed to start from:\n%s\n%s", r.manifest, r.out)
	}
	catalog := strings.Replace(pinsCatalog, `"3.3-9":`, `"3.3-7": {"metadata": {"airflowVersion": "3.3.1", "channel": "stable", "releaseDate": "2025-01-01", "pythonVersions": ["3.12"], "defaultPythonVersion": "3.12"}}, "3.3-9":`, 1)
	moved := strings.Replace(r.manifest, "apache-airflow==3.3.*", "apache-airflow==3.3.1", 1)
	f := &fakeUV{}
	got := pinsRun{
		src: moved, files: map[string]string{"uv.lock": r.lock}, catalog: catalog, uv: f,
		build:  runtimeversions.AstroBuild{Index: build332.Index, Airflow: "3.3.1+astro.1", TaskSDK: "1.3.1+astro.1"},
		freeze: []string{"apache-airflow==3.3.1+astro.1", "apache-airflow-task-sdk==1.3.1+astro.1", "pandas==2.2.0", "SQLAlchemy==2.0.40"},
		roots:  []string{"pandas"},
	}.start(t)
	if f.seedLocks != 1 {
		t.Errorf("a pin that moved off the seed's release did not re-seed (seed locks %d):\n%s", f.seedLocks, got.out)
	}
	if !strings.Contains(got.manifest, "astro seeded uv.lock from Astro Runtime 3.3-7") {
		t.Errorf("the record did not move with the seed:\n%s", got.manifest)
	}
}
