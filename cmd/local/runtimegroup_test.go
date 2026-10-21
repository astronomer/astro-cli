package local

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
)

var roots339 = []string{"apache-airflow-providers-celery", "pandas-market-calendars", "statsd"}

// A start writes the image's top-level packages into the astro-runtime group,
// of the build and Python the runtime pins are of, read from the package list
// the pins read, puts the group in default-groups, and says so once, with
// what it left out for want of a wheel.
func TestStartWritesTheRuntimeGroup(t *testing.T) {
	leftOut := []runtimecatalog.LeftOut{{Name: "apache-airflow-providers-mysql", Why: "it pulls in mysqlclient, which has no wheel on Linux x86_64, Linux aarch64 (Python 3.12)"}}
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339, leftOut: leftOut}.start(t)
	if strings.Join(r.rootsAsked, " ") != "3.3-9 3.12" || !slices.Equal(r.rootsFreeze, freeze339) {
		t.Errorf("roots asked for %q with %q, want the pins' build, Python and package list", r.rootsAsked, r.rootsFreeze)
	}
	for _, want := range []string{
		"[dependency-groups]",
		"# Astro Runtime 3.3-9 installs these",
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
	again := pinsRun{src: r.manifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if again.manifest != r.manifest || strings.Contains(again.out, "astro-runtime now installs") {
		t.Errorf("a second start with the same build changed the group:\n%s", again.out)
	}
}

// A start that cannot reach Astronomer's index keeps the Airflow build, and
// still brings the group up to date, which needs no index.
func TestStartRefreshesTheRuntimeGroupWithoutTheIndex(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, lookupErr: errors.New("dial tcp: no route to host"), freeze: freeze339, roots: []string{"statsd"}}.start(t)
	if !strings.Contains(r.manifest, "'statsd'") || strings.Contains(r.manifest, "'pandas-market-calendars'") {
		t.Errorf("the group was not refreshed without the index:\n%s", r.manifest)
	}
	if !strings.Contains(r.manifest, "'apache-airflow==3.3.2+astro.2'") {
		t.Errorf("the Airflow build did not stay:\n%s", r.manifest)
	}
}

// A list of top-level packages that cannot be had leaves the group as it is,
// with a warning.
func TestStartKeepsTheRuntimeGroupWhenTheListCannotBeRead(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freeze: freeze339, rootsErr: errors.New("uv lock failed")}.start(t)
	if r.manifest != written {
		t.Errorf("an unreadable list changed the group:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "keeps its astro-runtime dependency group as it is: uv lock failed") {
		t.Errorf("the start did not warn:\n%s", r.out)
	}
}

// Pins kept as they are, their own build's list unreadable, keep the group as
// it is too, without asking for its list.
func TestStartKeepsTheRuntimeGroupWithThePins(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: errors.New("dial tcp: no route to host"), roots: []string{"statsd"}}.start(t)
	if r.manifest != written || len(r.rootsAsked) != 0 {
		t.Errorf("the group changed (roots asked %q) beside pins kept as they are:\n%s", r.rootsAsked, r.manifest)
	}
}

// A build that publishes no package list takes the pins out, and empties the
// group, which stays with its place in default-groups.
func TestStartEmptiesTheRuntimeGroupOfABuildWithNoList(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: runtimecatalog.ErrFreezeNotPublished, roots: roots339}.start(t)
	if !emptied(r.manifest) || len(r.rootsAsked) != 0 {
		t.Errorf("the group was not emptied (roots asked %q) for a build with no list:\n%s", r.rootsAsked, r.manifest)
	}
	if !strings.Contains(r.out, "astro-runtime now lists none of a runtime image's packages") {
		t.Errorf("the start did not say it emptied the group:\n%s", r.out)
	}
}

// A list whose Airflow is another release than the Airflow pin's takes the
// pins out, and empties the group: both would be of an image the project
// does not build on.
func TestStartEmptiesTheRuntimeGroupWithThePinsOfAnotherRelease(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	other := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}
	r := pinsRun{src: written, build: build332, freeze: other, roots: roots339}.start(t)
	if !emptied(r.manifest) || len(r.rootsAsked) != 0 {
		t.Errorf("the group was not emptied (roots asked %q) beside pins of another release:\n%s", r.rootsAsked, r.manifest)
	}
}

// emptied reports a manifest whose astro-runtime group is there, empty.
func emptied(manifest string) bool {
	return strings.Contains(manifest, "# None apply to this project now.") && !strings.Contains(manifest, "'pandas-market-calendars'") &&
		strings.Contains(manifest, "'astro-runtime'")
}

// Under the project's own cutoff the pins come out, and the group, names
// alone, stays as it was: any cutoff resolves a name.
func TestStartKeepsTheRuntimeGroupUnderTheProjectsCutoff(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	src := strings.Replace(written, "[tool.uv]\n", "[tool.uv]\nexclude-newer = \"1 week\"\n", 1)
	r := pinsRun{src: src, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if len(r.rootsAsked) != 0 || !strings.Contains(r.manifest, "'pandas-market-calendars'") {
		t.Errorf("the list was read (%q), or the group dropped, under the project's cutoff:\n%s", r.rootsAsked, r.manifest)
	}
	if strings.Contains(r.manifest, "pandas==") {
		t.Errorf("the runtime pins stayed under the cutoff:\n%s", r.manifest)
	}
}
