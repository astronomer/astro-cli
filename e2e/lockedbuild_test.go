//go:build e2e && !windows

package e2e

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A packaged image starts FROM the runtime build the project's uv.lock was
// seeded from, not from the series tag.
//
// The record astro writes first in the [dependency-groups] astro-runtime group
// names the build the lock was seeded from, and every image is meant to be
// that build: otherwise the series tag moves on to a newer build and the
// image installs the lock's versions over a runtime they were not resolved
// for. So the build has to start FROM <build>-base, and the case reads that
// off the build's own log, where the base is named.
//
// The build named is the series' newest published one, which is the image the
// series tag serves today: the layers are the ones every other tier 3 case
// pulls, and only the tag differs, which is the thing under test.
func TestAPackagedImageBuildsOnTheBuildTheLockWasSeededFrom(t *testing.T) {
	tier(t, 3)
	p := dockerProject(t, "lockedbuild")
	needsDocker(t, p)

	series := strings.TrimPrefix(runtimeImageFor(t, p), "astrocrpublic.azurecr.io/runtime:")
	build := newestPublishedBuild(t, series)

	path := filepath.Join(p.Dir, "pyproject.toml")
	raw := read(t, path)
	// The suite runs uv under a cutoff, so init seeded nothing and wrote no
	// group: the record is written here, as a seed of that build would.
	if strings.Contains(raw, "[dependency-groups]") {
		t.Fatalf("init wrote a [dependency-groups] under the suite's cutoff:\n%s", raw)
	}
	write(t, path, raw+"\n[dependency-groups]\nastro-runtime = [\n"+
		"    # astro seeded uv.lock from Astro Runtime "+build+" (Python 3.12). Run astro init to seed it again.\n]\n")

	t.Cleanup(func() { removeImagesNamed(t, "astro-package/lockedbuild") })
	got := p.runSlow("package", "astro", "--output", "json").requireSuccess()

	// The exact build's -base tag, a Python flavor of it included; never the
	// series' own tag.
	exact := regexp.MustCompile(`runtime:` + regexp.QuoteMeta(build) + `(-python-\d+\.\d+)?-base\b`)
	if !exact.MatchString(got.output()) {
		t.Errorf("the build did not start FROM %s's -base image\n%s", build, got.output())
	}
	t.Logf("built FROM %s", exact.FindString(got.output()))
	if strings.Contains(got.output(), "runtime:"+series+"-base") {
		t.Errorf("the build started FROM the series tag %s-base, not the build the lock was seeded from\n%s", series, got.output())
	}
}

// newestPublishedBuild is the newest non-yanked build of an Airflow 3 series
// the runtime catalog lists as released today, read from the catalog itself:
// this module imports nothing under test (doc.go).
func newestPublishedBuild(t *testing.T, series string) string {
	t.Helper()
	ctx := t.Context()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://updates.astronomer.io/astronomer-runtime", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("reading the runtime catalog: %v", err)
	}
	defer resp.Body.Close()
	var catalog struct {
		V3 map[string]struct {
			Metadata struct {
				Channel     string `json:"channel"`
				ReleaseDate string `json:"releaseDate"`
				Yanked      bool   `json:"yanked"`
			} `json:"metadata"`
		} `json:"runtimeVersionsV3"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&catalog); err != nil {
		t.Fatalf("decoding the runtime catalog: %v", err)
	}
	today := time.Now().UTC().Format("2006-01-02")
	newest, newestN := "", -1
	for tag, r := range catalog.V3 {
		s, n, ok := strings.Cut(tag, "-")
		if !ok || s != series || r.Metadata.Yanked || r.Metadata.Channel != "stable" || r.Metadata.ReleaseDate > today {
			continue
		}
		if i, err := strconv.Atoi(n); err == nil && i > newestN {
			newest, newestN = tag, i
		}
	}
	if newest == "" {
		t.Fatalf("the runtime catalog lists no released build of Airflow %s", series)
	}
	return newest
}
