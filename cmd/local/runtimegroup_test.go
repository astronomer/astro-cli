package local

import (
	"errors"
	"strings"
	"testing"

	"github.com/astronomer/astro-cli/internal/runtimecatalog"
)

var roots339 = []string{"apache-airflow-providers-celery", "pandas-market-calendars", "statsd"}

// A start writes the image's top-level packages into the astro-runtime group,
// of the build and Python the runtime pins are of, puts the group in
// default-groups, and says so once.
func TestStartWritesTheRuntimeGroup(t *testing.T) {
	r := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if strings.Join(r.rootsAsked, " ") != "3.3-9 3.12" {
		t.Errorf("roots asked for %q, want the pins' build and Python", r.rootsAsked)
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
	if !strings.Contains(r.out, "astro-runtime now installs the 3 packages Astro Runtime 3.3-9 installs") {
		t.Errorf("the start did not say it wrote the group:\n%s", r.out)
	}
	again := pinsRun{src: r.manifest, build: build332, freeze: freeze339, roots: roots339}.start(t)
	if again.manifest != r.manifest || strings.Contains(again.out, "astro-runtime now installs") {
		t.Errorf("a second start with the same build changed the group:\n%s", again.out)
	}
}

// A list that cannot be worked out leaves the group as it is, with a warning.
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

// A build that publishes no freeze has no list: the group goes, with its place
// in default-groups.
func TestStartTakesOutTheRuntimeGroupOfABuildWithNoList(t *testing.T) {
	written := pinsRun{src: buildManifest, build: build332, freeze: freeze339, roots: roots339}.start(t).manifest
	r := pinsRun{src: written, build: build332, freezeErr: runtimecatalog.ErrFreezeNotPublished, rootsErr: runtimecatalog.ErrFreezeNotPublished}.start(t)
	if strings.Contains(r.manifest, "astro-runtime") || strings.Contains(r.manifest, "dependency-groups") {
		t.Errorf("the group stayed for a build with no list:\n%s", r.manifest)
	}
	if !strings.Contains(r.out, "astro-runtime is gone") {
		t.Errorf("the start did not say it took the group out:\n%s", r.out)
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
