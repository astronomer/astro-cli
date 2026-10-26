package local

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
)

var roots339 = []string{"apache-airflow-providers-celery", "pandas-market-calendars", "statsd"}

// A start writes the image's top-level packages into the astro-runtime group,
// of the build and Python the lock is seeded from, read from the package list
// the seed reads, puts the group in default-groups, records the seed first in
// it, and says so once, with what it left out for want of a wheel.
func TestStartWritesTheRuntimeGroup(t *testing.T) {
	leftOut := []runtimecatalog.LeftOut{{Name: "apache-airflow-providers-mysql", Why: "it pulls in mysqlclient, which has no wheel on Linux x86_64, Linux aarch64 (Python 3.12)"}}
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339, leftOut: leftOut}.start(t)
	if strings.Join(r.rootsAsked, " ") != "3.3-9 3.12" || !slices.Equal(r.rootsFreeze, freeze339) {
		t.Errorf("roots asked for %q with %q, want the seed's build, Python and package list", r.rootsAsked, r.rootsFreeze)
	}
	for _, want := range []string{
		"[dependency-groups]",
		"    # astro seeded uv.lock from Astro Runtime 3.3-9 (Python 3.12).",
		"'pandas-market-calendars'",
		"'astro-runtime'",
	} {
		if !strings.Contains(r.manifest, want) {
			t.Errorf("manifest lacks %q:\n%s", want, r.manifest)
		}
	}
	for _, want := range []string{
		"astro-runtime now installs the 3 packages Astro Runtime 3.3-9 installs",
		"astro-runtime leaves out apache-airflow-providers-mysql, which the runtime image has: it pulls in mysqlclient, which has no wheel on Linux x86_64, Linux aarch64 (Python 3.12). Docker mode still has it; uv add apache-airflow-providers-mysql installs it anyway",
	} {
		if !strings.Contains(r.out, want) {
			t.Errorf("the start did not say %q:\n%s", want, r.out)
		}
	}
	again := pinsRun{src: r.manifest, files: map[string]string{"uv.lock": r.lock}, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if again.manifest != r.manifest || strings.Contains(again.out, "astro-runtime now installs") {
		t.Errorf("a second start with the same build changed the group:\n%s", again.out)
	}
}

// What the group left out is said only of a group that was written: not of
// one refused, or a write that failed.
func TestTheLeftOutNoteNeedsTheGroupWritten(t *testing.T) {
	d, _ := testDeps(t)
	stderr := &bytes.Buffer{}
	d.Stderr = stderr
	c := &cli{d: d}
	res := astroBuildWrite{groupLeftOut: []runtimecatalog.LeftOut{{Name: "mysqlclient", Why: "it has no wheel"}}}
	c.reportGroupLeftOut(res)
	if stderr.Len() != 0 {
		t.Errorf("a group not written was reported:\n%s", stderr)
	}
	res.groupWritten = true
	c.reportGroupLeftOut(res)
	if !strings.Contains(stderr.String(), "astro-runtime leaves out mysqlclient") {
		t.Errorf("a written group's left-out package was not reported:\n%s", stderr)
	}
}

// A start that cannot reach Astronomer's index keeps the Airflow build, and
// still brings the group up to date, which needs no index, and seeds the
// lock again for the packages new in it.
func TestStartRefreshesTheRuntimeGroupWithoutTheIndex(t *testing.T) {
	first := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	r := pinsRun{src: first.manifest, files: map[string]string{"uv.lock": first.lock}, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339, roots: []string{"statsd", "pandas"}}.start(t)
	if !strings.Contains(r.manifest, "'statsd'") || strings.Contains(r.manifest, "'pandas-market-calendars'") {
		t.Errorf("the group was not refreshed without the index:\n%s", r.manifest)
	}
	if r.uv.scratchLocks != 1 {
		t.Errorf("a group with new packages in it did not seed the lock again (scratch locks %d)", r.uv.scratchLocks)
	}
	if !strings.Contains(r.manifest, "'apache-airflow==3.3.2+astro.2'") {
		t.Errorf("the Airflow build did not stay:\n%s", r.manifest)
	}
}

// A list of top-level packages that cannot be had leaves the group as it is,
// with a warning.
func TestStartKeepsTheRuntimeGroupWhenTheListCannotBeRead(t *testing.T) {
	first := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	written := first.manifest
	r := pinsRun{src: written, files: map[string]string{"uv.lock": first.lock}, build: build332, freeze: freeze339, rootsErr: errors.New("uv lock failed")}.start(t)
	if r.manifest != written {
		t.Errorf("an unreadable list changed the group:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "keeps its astro-runtime dependency group as it is: uv lock failed") {
		t.Errorf("the start did not warn:\n%s", r.out)
	}
}

// A seed kept as it is, its own build's list unreadable, keeps the group as
// it is too, without asking for its list.
func TestStartKeepsTheRuntimeGroupWithThePins(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: errors.New("dial tcp: no route to host"), roots: []string{"statsd"}}.start(t)
	if r.manifest != written || len(r.rootsAsked) != 0 {
		t.Errorf("the group changed (roots asked %q) beside a seed kept as it is:\n%s", r.rootsAsked, r.manifest)
	}
}

// A build that publishes no package list seeds nothing, takes the record out,
// and empties the group, which stays with its place in default-groups.
func TestStartEmptiesTheRuntimeGroupOfABuildWithNoList(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: runtimecatalog.ErrFreezeNotPublished, roots: roots339}.start(t)
	if !emptied(r.manifest) || len(r.rootsAsked) != 0 || strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("the group was not emptied, or its record kept (roots asked %q), for a build with no list:\n%s", r.rootsAsked, r.manifest)
	}
	if !strings.Contains(r.out, "astro-runtime now lists none of a runtime image's packages") {
		t.Errorf("the start did not say it emptied the group:\n%s", r.out)
	}
}

// A list whose Airflow is another release than the Airflow pin's seeds
// nothing, and empties the group: both would be of an image the project does
// not build on.
func TestStartEmptiesTheRuntimeGroupWithThePinsOfAnotherRelease(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	other := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}
	r := pinsRun{src: written, build: build332, freeze: other, roots: roots339}.start(t)
	if !emptied(r.manifest) || len(r.rootsAsked) != 0 {
		t.Errorf("the group was not emptied (roots asked %q) for another release's list:\n%s", r.rootsAsked, r.manifest)
	}
}

// emptied reports a manifest whose astro-runtime group is there, empty.
func emptied(manifest string) bool {
	return strings.Contains(manifest, "# None apply to this project now.") && !strings.Contains(manifest, "'pandas-market-calendars'") &&
		strings.Contains(manifest, "'astro-runtime'")
}

// Under the project's own cutoff nothing is seeded and the record comes out,
// and the group, names alone, stays as it was: any cutoff resolves a name.
func TestStartKeepsTheRuntimeGroupUnderTheProjectsCutoff(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	src := strings.Replace(written, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	r := pinsRun{src: src, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if len(r.rootsAsked) != 0 || !strings.Contains(r.manifest, "'pandas-market-calendars'") {
		t.Errorf("the list was read (%q), or the group dropped, under the project's cutoff:\n%s", r.rootsAsked, r.manifest)
	}
	if strings.Contains(r.manifest, "astro seeded") {
		t.Errorf("the record stayed under the cutoff:\n%s", r.manifest)
	}
}
