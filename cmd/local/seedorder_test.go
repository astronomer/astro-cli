package local

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

// requires reports whether m's [project] dependencies or its astro-runtime
// group name dist: what a lock holds, since a constraint adds no package.
func requires(m *manifest.Manifest, dist string) bool {
	for _, d := range slices.Concat(m.Project.Dependencies, m.RuntimeGroup) {
		if manifest.DistName(d) == dist {
			return true
		}
	}
	return false
}

// The group is in pyproject.toml while the lock is seeded, so the lock holds
// its packages at the image's versions: a lock seeded without them would
// leave them to the newest on the next sync.
func TestTheSeedHoldsTheGroupsPackages(t *testing.T) {
	freeze := append(slices.Clone(freeze339), "statsd==4.0.1")
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze, roots: roots339, uv: &fakeUV{requiredOnly: true}}.start(t)
	if !strings.Contains(r.manifest, "'statsd'") {
		t.Fatalf("the group was not written:\n%s", r.manifest)
	}
	if !strings.Contains(r.lock, "name = \"statsd\"\nversion = \"4.0.1\"") {
		t.Errorf("the seeded lock does not hold the group's statsd at the image's version:\n%s", r.lock)
	}
}

// A seed that fails leaves the group, its record and the lock exactly as they
// were: the group of a new build is written only once its seed has worked.
func TestAFailedSeedLeavesTheGroupAndRecordAsTheyWere(t *testing.T) {
	first := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	r := pinsRun{
		src: first.manifest, files: map[string]string{"uv.lock": first.lock}, build: build332, freeze: freeze339,
		roots: []string{"statsd", "redis"}, init: true, uv: &fakeUV{err: errors.New("no solution found")},
	}.start(t)
	if r.manifest != first.manifest || r.lock != first.lock {
		t.Errorf("a failed seed changed the group, its record or the lock:\n%s", r.manifest)
	}
}

// A group that only loses a package, because the project now lists it, needs
// no seed; one that gains a package does.
func TestOnlyAGroupThatGainsAPackageIsSeededAgain(t *testing.T) {
	first := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	listed := strings.Replace(first.manifest, "'apache-airflow-task-sdk',", "'apache-airflow-task-sdk',\n    'statsd',", 1)
	r := pinsRun{src: listed, files: map[string]string{"uv.lock": first.lock}, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if r.uv.seedLocks != 0 {
		t.Errorf("a group that only lost a package was seeded again (%d seed locks)", r.uv.seedLocks)
	}
	if !strings.Contains(r.manifest, "'pandas-market-calendars'") || strings.Count(r.manifest, "'statsd'") != 1 {
		t.Errorf("the group did not drop only what the project lists:\n%s", r.manifest)
	}
	gained := pinsRun{
		src: first.manifest, files: map[string]string{"uv.lock": first.lock}, build: build332, freeze: freeze339,
		roots: append(slices.Clone(roots339), "redis"),
	}.start(t)
	if gained.uv.seedLocks != 1 || !strings.Contains(gained.manifest, "'redis'") {
		t.Errorf("a group that gained a package was not seeded again (%d seed locks):\n%s", gained.uv.seedLocks, gained.manifest)
	}
}
