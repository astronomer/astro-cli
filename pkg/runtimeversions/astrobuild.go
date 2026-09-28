package runtimeversions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultIndexURL is Astronomer's public package index, the one the Astro
	// Runtime image installs Airflow from. It needs no credentials.
	DefaultIndexURL = "https://pip.astronomer.io/v2/"
	// IndexURLEnv overrides DefaultIndexURL, for a mirror or a test server.
	IndexURLEnv = "ASTRO_AIRFLOW_INDEX_URL"

	// indexTTL is how long a cached index page is used without asking again.
	indexTTL = 24 * time.Hour
)

// The distributions Astronomer rebuilds for Airflow 3. Airflow 2 has only the
// first.
const (
	DistAirflow = "apache-airflow"
	DistCore    = "apache-airflow-core"
	DistTaskSDK = "apache-airflow-task-sdk"
)

// ErrNoAstroBuild reports an Airflow pin Astronomer's index has no build of,
// a release newer than any runtime most often. Airflow then comes from PyPI.
var ErrNoAstroBuild = errors.New("Astronomer's index has no build of this Airflow")

// AstroBuild is the Astronomer build of Airflow a deployment runs, as its
// runtime image pins it.
type AstroBuild struct {
	// Index is the package index the build is on.
	Index string
	// Airflow is the build of apache-airflow, and of apache-airflow-core on
	// Airflow 3: "3.3.2+astro.1".
	Airflow string
	// TaskSDK is the Task SDK build that ships with it, "1.3.2+astro.1".
	// Empty on Airflow 2.
	TaskSDK string
}

// Dists are the distributions the build covers.
func (b AstroBuild) Dists() []string {
	if b.TaskSDK == "" {
		return []string{DistAirflow}
	}
	return []string{DistAirflow, DistCore, DistTaskSDK}
}

// Pins are the constraints that hold a project to the build: the lines the
// runtime image's own pip constraints carry for Airflow and the Task SDK.
func (b AstroBuild) Pins() []string {
	pins := []string{DistAirflow + "==" + b.Airflow}
	if b.TaskSDK != "" {
		pins = append(pins, DistTaskSDK+"=="+b.TaskSDK)
	}
	return pins
}

// LookupAstroBuild finds the Astronomer build of the Airflow a deployment of
// this pin ("3.3" or "3.3.2") and [tool.astro] runtime build runs.
//
// The release is the catalog's: the one the runtime build carries, or the one
// the newest runtime of the pin carries. The catalog does not say which
// +astro.N build that runtime pins, so the newest build of the release on the
// index is taken. Without the catalog, or when its release has no build, it
// is the newest build the pin covers.
//
// Builds published after excludeNewer are left out, and the catalog is not
// read then, since it knows only the present.
//
// ErrNoAstroBuild means the index was read and has no build the pin covers.
// Any other error means the index could not be read, so nothing is known.
func LookupAstroBuild(ctx context.Context, o Options, pin, runtime string, excludeNewer time.Time) (AstroBuild, error) {
	page, err := o.indexPage(ctx, DistAirflow)
	if err != nil {
		return AstroBuild{}, err
	}
	var covered []astroBuild
	for _, b := range parseBuilds(page, DistAirflow, excludeNewer) {
		if pinCovers(pin, b.release) {
			covered = append(covered, b)
		}
	}
	if excludeNewer.IsZero() {
		if catalog, _, err := Load(ctx, o); err == nil {
			if release, ok := catalog.AirflowFor(pin, runtime); ok {
				if of := buildsOf(covered, release); len(of) > 0 {
					covered = of
				}
			}
		}
	}
	chosen, ok := newestBuild(covered)
	if !ok {
		return AstroBuild{}, fmt.Errorf("%w: apache-airflow==%s", ErrNoAstroBuild, pin)
	}
	out := AstroBuild{Index: PublicIndexURL(), Airflow: chosen.version()}
	if majorOf(chosen.release) == "2" {
		return out, nil
	}
	sdk, err := o.taskSDK(ctx, chosen.release, excludeNewer)
	if err != nil {
		return AstroBuild{}, err
	}
	out.TaskSDK = sdk
	return out, nil
}

// taskSDK is the Task SDK build that ships with an Airflow 3 release. Airflow
// releases the two in step, 3.3.2 with 1.3.2, and Astronomer's build of
// Airflow requires the SDK without a version, so without a pin the newest SDK
// on the index would be taken whichever Airflow it belongs to.
//
// With no build of that SDK to name, nothing is known well enough to write,
// so it reports an error rather than a pin that could not be told apart from
// one the project wrote itself.
func (o Options) taskSDK(ctx context.Context, airflowRelease string, excludeNewer time.Time) (string, error) {
	_, minorPatch, _ := strings.Cut(airflowRelease, ".")
	release := "1." + minorPatch
	page, err := o.indexPage(ctx, DistTaskSDK)
	if err != nil {
		return "", err
	}
	if b, ok := newestBuild(buildsOf(parseBuilds(page, DistTaskSDK, excludeNewer), release)); ok {
		return b.version(), nil
	}
	return "", fmt.Errorf("Astronomer's index has no build of the Task SDK %s that ships with Airflow %s", release, airflowRelease)
}

// ParseExcludeNewer reads a UV_EXCLUDE_NEWER value in either form uv takes as
// a point in time, an RFC 3339 time or a plain day. Anything else, a relative
// duration included, reads as unset: a cutoff that moves with the clock hides
// nothing a lookup made now could pick, since a new build is also the newest.
func ParseExcludeNewer(v string) time.Time {
	v = strings.TrimSpace(v)
	for _, layout := range []string{time.RFC3339, time.DateOnly} {
		if t, err := time.Parse(layout, v); err == nil {
			return t
		}
	}
	return time.Time{}
}

// astroBuild is one build on the index: release 3.3.2, n 1 for 3.3.2+astro.1.
type astroBuild struct {
	release   string
	n         int
	published time.Time
}

func (b astroBuild) version() string { return b.release + "+astro." + strconv.Itoa(b.n) }

// fileRe matches a final release's file on the index, a wheel or an sdist:
// "apache_airflow-3.3.2+astro.1-py3-none-any.whl". Pre-releases and dev builds
// do not match, so are never chosen.
var fileRe = regexp.MustCompile(`^([a-z_]+)-(\d+\.\d+\.\d+)\+astro\.(\d+)(?:-py3-none-any\.whl|\.tar\.gz)$`)

// linkRe matches one link and the upload time the index lists ahead of it on
// the same line, when it lists one.
var linkRe = regexp.MustCompile(`(?:(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z)[^<]*)?<a href="([^"]+)"`)

// parseBuilds reads the builds of dist off its index page. A file listed with
// an upload time after excludeNewer is left out; one with no time is kept.
func parseBuilds(page []byte, dist string, excludeNewer time.Time) []astroBuild {
	prefix := strings.ReplaceAll(dist, "-", "_")
	var out []astroBuild
	for _, m := range linkRe.FindAllStringSubmatch(string(page), -1) {
		file := m[2]
		if i := strings.IndexAny(file, "#?"); i >= 0 {
			file = file[:i]
		}
		file = file[strings.LastIndex(file, "/")+1:]
		f := fileRe.FindStringSubmatch(file)
		if len(f) == 0 || f[1] != prefix {
			continue
		}
		n, err := strconv.Atoi(f[3])
		if err != nil {
			continue
		}
		b := astroBuild{release: f[2], n: n}
		if m[1] != "" {
			b.published, _ = time.Parse(time.RFC3339, m[1]) //nolint:errcheck // the pattern already matched RFC 3339
		}
		if !excludeNewer.IsZero() && b.published.After(excludeNewer) {
			continue
		}
		out = append(out, b)
	}
	return out
}

func buildsOf(builds []astroBuild, release string) []astroBuild {
	var out []astroBuild
	for _, b := range builds {
		if b.release == release {
			out = append(out, b)
		}
	}
	return out
}

func newestBuild(builds []astroBuild) (astroBuild, bool) {
	var best astroBuild
	found := false
	for _, b := range builds {
		if c := compareVersions(b.release, best.release); !found || c > 0 || (c == 0 && b.n > best.n) {
			best, found = b, true
		}
	}
	return best, found
}

// IndexURL is the index builds are looked up on: IndexURLEnv when set, else
// DefaultIndexURL.
func IndexURL() string {
	if u := strings.TrimSpace(os.Getenv(IndexURLEnv)); u != "" {
		return strings.TrimSuffix(u, "/") + "/"
	}
	return DefaultIndexURL
}

// PublicIndexURL is IndexURL without any user or token in it, for what gets
// written into a project's pyproject.toml or printed. uv reads a mirror's
// credentials from its own settings, not from the committed file.
func PublicIndexURL() string {
	return withoutUser(IndexURL())
}

// withoutUser is rawURL without its user and password, for an address that is
// written or printed.
func withoutUser(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

// indexPage returns dist's page on the index: a cached copy younger than a
// day, else a fetch, else a cached copy of any age. A page that lists no
// build is never kept: a captive portal answers 200 too, and a copy of its
// page would stand in for the index for a day.
func (o Options) indexPage(ctx context.Context, dist string) ([]byte, error) {
	url := IndexURL() + dist + "/"
	path := ""
	if o.CacheDir != "" {
		sum := sha256.Sum256([]byte(url))
		path = filepath.Join(o.CacheDir, "airflow-index", dist+"-"+hex.EncodeToString(sum[:])[:12]+".html")
		if data, err := readFresh(path, indexTTL); err == nil {
			return data, nil
		}
	}
	data, err := fetch(ctx, o, url, "text/html")
	if err == nil && len(parseBuilds(data, dist, time.Time{})) == 0 {
		err = fmt.Errorf("%s lists no Astronomer builds", withoutUser(url))
	}
	if err == nil {
		if path != "" {
			writeCache(path, data)
		}
		return data, nil
	}
	if path != "" {
		if cached, rerr := os.ReadFile(path); rerr == nil {
			return cached, nil
		}
	}
	return nil, fmt.Errorf("reading Astronomer's index: %w", err)
}
