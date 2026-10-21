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
	leftOut := []runtimecatalog.LeftOut{{Name: "apache-airflow-providers-mysql", Why: "it pulls in mysqlclient, which has no wheel for Linux x86_64, Linux aarch64"}}
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
		"astro-runtime leaves out apache-airflow-providers-mysql, which the runtime image has: it pulls in mysqlclient, which has no wheel for Linux x86_64, Linux aarch64 on Python 3.12. Docker mode still has it; uv add apache-airflow-providers-mysql installs it anyway",
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

// A build that publishes no package list takes the pins out, and the group
// with them, with its place in default-groups.
func TestStartTakesOutTheRuntimeGroupOfABuildWithNoList(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: runtimecatalog.ErrFreezeNotPublished, roots: roots339}.start(t)
	if strings.Contains(r.manifest, "astro-runtime") || strings.Contains(r.manifest, "dependency-groups") || len(r.rootsAsked) != 0 {
		t.Errorf("the group stayed (roots asked %q) for a build with no list:\n%s", r.rootsAsked, r.manifest)
	}
	if !strings.Contains(r.out, "astro-runtime is gone") {
		t.Errorf("the start did not say it took the group out:\n%s", r.out)
	}
}

// A list whose Airflow is another release than the Airflow pin's takes the
// pins out, and the group with them: both would be of an image the project
// does not build on.
func TestStartTakesOutTheRuntimeGroupWithThePinsOfAnotherRelease(t *testing.T) {
	// A runtime the user named, carrying another release than the requirement,
	// takes the pins out, and the group with them.
	named := strings.Replace(buildManifest, "[tool.astro]\n", "[tool.astro]\nruntime = '3.3-9'\n", 1)
	written := pinsRun{src: named, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	other := []string{"apache-airflow==3.3.1+astro.1", "pandas==2.2.3"}
	r := pinsRun{src: written, build: build332, freeze: other, roots: roots339}.start(t)
	if strings.Contains(r.manifest, "astro-runtime =") || len(r.rootsAsked) != 0 {
		t.Errorf("the group stayed (roots asked %q) beside pins of another release:\n%s", r.rootsAsked, r.manifest)
	}

	// With no build named, the lookup and the list disagree on their own: the
	// pins there stay, and the group with them.
	written = pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r = pinsRun{src: written, build: build332, freeze: other, roots: roots339}.start(t)
	if !strings.Contains(r.manifest, "# Astro Runtime 3.3-9 installs these") || len(r.rootsAsked) != 0 {
		t.Errorf("kept pins did not keep the group (roots asked %q):\n%s", r.rootsAsked, r.manifest)
	}
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

// The group follows what happens to the runtime pins. A new build whose list
// cannot be read keeps the pins of the build there, and the group with them.
// astro init in a project that already is one moves the pins to the newest
// build, the group with them, and relocks so uv.lock follows both.
func TestTheRuntimeGroupFollowsKeptAndMovedPins(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	older := strings.NewReplacer(
		"# Astro Runtime 3.3-9 (Python 3.12)", "# Astro Runtime 3.3-8 (Python 3.12)",
		"# Astro Runtime 3.3-9 installs these", "# Astro Runtime 3.3-8 installs these",
	).Replace(written)

	kept := pinsRun{src: older, build: build332, freezeErr: errors.New("offline"), roots: []string{"statsd"}}.start(t)
	for _, want := range []string{"# Astro Runtime 3.3-8 (Python 3.12)", "# Astro Runtime 3.3-8 installs these", "'pandas-market-calendars'"} {
		if !strings.Contains(kept.manifest, want) {
			t.Errorf("kept pins did not keep the group (%q missing):\n%s", want, kept.manifest)
		}
	}

	var relocked int
	moved := pinsRun{
		src: older, build: build332, freeze: freeze339, roots: []string{"statsd"}, args: []string{"init"},
		relock: func(string) error { relocked++; return nil }, files: map[string]string{"uv.lock": "version = 1\n"},
	}.start(t)
	if !strings.Contains(moved.manifest, "# Astro Runtime 3.3-9 installs these") || !strings.Contains(moved.manifest, "'statsd'") ||
		strings.Contains(moved.manifest, "'pandas-market-calendars'") {
		t.Errorf("init did not move the group with the pins:\n%s", moved.manifest)
	}
	if relocked != 1 {
		t.Errorf("uv lock ran %d times after init moved the pins and the group, want once", relocked)
	}
}
