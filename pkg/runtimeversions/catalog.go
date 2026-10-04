// Package runtimeversions reads Astronomer's Astro Runtime catalog, the public
// document at https://updates.astronomer.io/astronomer-runtime that says which
// runtimes exist, which Airflow each carries, which are yanked, and which Python
// versions each ships.
//
// It owns fetching, caching and decoding, and the few questions its consumers
// ask of it: which Airflow series a new project should start on, which Python
// that series runs, which runtime carries a given Airflow, and, from
// Astronomer's package index beside the catalog, which build of that Airflow
// the runtime installs (LookupAstroBuild). astro init, the
// Airflow 2 image lookup in pkg/imagebuild and Astro Desktop's runtime poller
// all read it through here, so there is one answer to each.
//
// Nothing here prints, and nothing here fails a caller that has a fallback:
// Default never returns an error, and Load returns one only when there is no
// copy of the catalog anywhere.
package runtimeversions

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// channelStable is the one channel a new project may start on. The catalog
// also uses "deprecated", which retires a whole runtime line.
const channelStable = "stable"

// releaseDateLayout is how the catalog writes a release date.
const releaseDateLayout = "2006-01-02"

// Runtime is one Astro Runtime build as the catalog describes it.
type Runtime struct {
	// Tag is the runtime version: "3.3-8" for Airflow 3, "13.11.0" for
	// Airflow 2.
	Tag string
	// AirflowVersion is the exact Airflow the build carries, like "3.3.2".
	AirflowVersion string
	// Channel is "stable" or "deprecated".
	Channel string
	// ReleaseDate is the catalog's date, "2026-09-23", or empty.
	ReleaseDate string
	// Yanked marks one build withdrawn, and YankedReason says why.
	Yanked       bool
	YankedReason string
	// PythonVersions lists the interpreters the image ships, like
	// ["3.12", "3.13", "3.14"]. Airflow 2 builds list none.
	PythonVersions       []string
	DefaultPythonVersion string
}

// Catalog is a decoded copy of the catalog.
type Catalog struct {
	// runtimes holds both of the document's maps, keyed by tag. The two tag
	// shapes ("13.11.0", "3.3-8") cannot collide, and every query filters by
	// the Airflow a build carries rather than by the map it came from.
	runtimes map[string]*Runtime
}

// document is the slice of the catalog this package reads. Unknown fields are
// ignored, so the schema can grow without breaking a released binary.
type document struct {
	RuntimeVersions   map[string]entry `json:"runtimeVersions"`
	RuntimeVersionsV3 map[string]entry `json:"runtimeVersionsV3"`
}

type entry struct {
	Metadata struct {
		AirflowVersion       string   `json:"airflowVersion"`
		Channel              string   `json:"channel"`
		ReleaseDate          string   `json:"releaseDate"`
		Yanked               bool     `json:"yanked"`
		YankedReason         string   `json:"yankedReason"`
		PythonVersions       []string `json:"pythonVersions"`
		DefaultPythonVersion string   `json:"defaultPythonVersion"`
	} `json:"metadata"`
}

// errEmpty reports a document that decoded and named no runtime, which is not a
// catalog anyone should act on or cache.
var errEmpty = errors.New("the version service listed no runtimes")

// now is the clock the release-date rule reads. A var so tests can stand at a
// fixed day.
var now = time.Now

// Parse decodes the catalog's raw bytes, as fetched or as cached.
func Parse(data []byte) (*Catalog, error) {
	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	c := &Catalog{runtimes: make(map[string]*Runtime, len(doc.RuntimeVersions)+len(doc.RuntimeVersionsV3))}
	for _, m := range []map[string]entry{doc.RuntimeVersions, doc.RuntimeVersionsV3} {
		for tag, e := range m {
			md := e.Metadata
			c.runtimes[tag] = &Runtime{
				Tag:                  tag,
				AirflowVersion:       md.AirflowVersion,
				Channel:              md.Channel,
				ReleaseDate:          md.ReleaseDate,
				Yanked:               md.Yanked,
				YankedReason:         md.YankedReason,
				PythonVersions:       md.PythonVersions,
				DefaultPythonVersion: md.DefaultPythonVersion,
			}
		}
	}
	if len(c.runtimes) == 0 {
		return nil, errEmpty
	}
	return c, nil
}

// Runtime returns the build a tag names.
func (c *Catalog) Runtime(tag string) (Runtime, bool) {
	r, ok := c.runtimes[tag]
	if !ok {
		return Runtime{}, false
	}
	return *r, true
}

// LatestAirflowSeries is the highest Airflow series of a generation ("3") with
// at least one build that is stable, not yanked, and released on or before
// today. The answer is a series, "3.3", never a patch.
//
// Highest by version, not by date: older lines keep shipping backports after a
// newer one exists (3.0-18 shipped two months after 3.3-1), so the most
// recently released build says nothing about which series is newest.
//
// A yanked build retires only itself, so a series whose first build was yanked
// still qualifies through its next one. There is no cooldown: stable is
// Astronomer's GA gate, and holding a series back is done in the catalog, by
// yanking or deprecating its builds.
func (c *Catalog) LatestAirflowSeries(major string) (string, bool) {
	var best string
	for _, r := range c.runtimes {
		if !r.qualifies() {
			continue
		}
		s := seriesOf(r.AirflowVersion)
		if s == "" || majorOf(s) != major {
			continue
		}
		if best == "" || compareVersions(s, best) > 0 {
			best = s
		}
	}
	return best, best != ""
}

// PythonVersions returns the interpreters an Airflow series' runtime ships,
// read from its newest qualifying build that lists any. Airflow 2 builds list
// none, so an Airflow 2 series always reports false.
func (c *Catalog) PythonVersions(series string) ([]string, bool) {
	var best *Runtime
	for _, r := range c.runtimes {
		if !r.qualifies() || len(r.PythonVersions) == 0 || seriesOf(r.AirflowVersion) != series {
			continue
		}
		if best == nil || compareVersions(r.Tag, best.Tag) > 0 {
			best = r
		}
	}
	if best == nil {
		return nil, false
	}
	return append([]string(nil), best.PythonVersions...), true
}

// RequiresPython is the [project] requires-python a new project on this
// series gets: ">=" and the lowest Python its runtime ships, with no upper
// bound. Airflow 3 tracks new interpreters, and a ceiling would refuse a Python
// that works. The floor is what matters, because it keeps standalone mode on an
// interpreter the runtime image also has. Gaps in the list are ignored.
//
// It reports false when the catalog lists no Python for the series, and the
// caller then applies its own built-in rule.
func (c *Catalog) RequiresPython(series string) (string, bool) {
	versions, ok := c.PythonVersions(series)
	if !ok {
		return "", false
	}
	var lowest string
	for _, v := range versions {
		v = strings.TrimSpace(v)
		if !numericVersion(v) {
			continue
		}
		if lowest == "" || compareVersions(v, lowest) < 0 {
			lowest = v
		}
	}
	if lowest == "" {
		return "", false
	}
	return ">=" + lowest, true
}

// NewestRuntimeFor returns the newest build, by tag, whose Airflow the pin
// covers: the same version, or any version under a partial pin ("2", "2.9").
// Yanked builds are skipped. Channel and release date are not consulted: a
// project pinned to a deprecated line still needs an image to run.
func (c *Catalog) NewestRuntimeFor(airflowPin string) (string, bool) {
	var best string
	for tag, r := range c.runtimes {
		if r.Yanked || !pinCovers(airflowPin, r.AirflowVersion) {
			continue
		}
		if best == "" || compareVersions(tag, best) > 0 {
			best = tag
		}
	}
	return best, best != ""
}

// NewestPublishedRuntimeFor is NewestRuntimeFor less the builds the catalog
// dates after today: the build a series tag such as runtime:3.3 serves, whose
// own tags (runtime:3.3-8-python-3.13) a caller can pull. A build listed ahead
// of its release has no image yet. A build with no date, or one that does not
// parse, still counts, as it does for NewestRuntimeFor.
func (c *Catalog) NewestPublishedRuntimeFor(airflowPin string) (string, bool) {
	var best string
	for tag, r := range c.runtimes {
		if r.Yanked || r.scheduled() || !pinCovers(airflowPin, r.AirflowVersion) {
			continue
		}
		if best == "" || compareVersions(tag, best) > 0 {
			best = tag
		}
	}
	return best, best != ""
}

// AirflowFor is the exact Airflow a deployment of this project runs: the one
// the runtime build carries when the manifest names one, and otherwise the one
// the newest build of the pin carries, which is what the series image tag
// resolves to.
func (c *Catalog) AirflowFor(airflowPin, runtime string) (string, bool) {
	tag := runtime
	if tag == "" {
		var ok bool
		if tag, ok = c.NewestRuntimeFor(airflowPin); !ok {
			return "", false
		}
	}
	r, ok := c.runtimes[tag]
	if !ok || r.AirflowVersion == "" {
		return "", false
	}
	return r.AirflowVersion, true
}

// qualifies is the test of one build: stable, not yanked, released.
func (r *Runtime) qualifies() bool {
	return r.Channel == channelStable && !r.Yanked && r.released()
}

// released reports a release date on or before today, in UTC. A build with no
// date, or one that does not parse, is not counted: nothing says it has
// shipped.
func (r *Runtime) released() bool {
	day, err := time.Parse(releaseDateLayout, r.ReleaseDate)
	if err != nil {
		return false
	}
	return !day.After(now().UTC())
}

// scheduled reports a release date after today, in UTC: a build the catalog
// lists before it ships.
func (r *Runtime) scheduled() bool {
	day, err := time.Parse(releaseDateLayout, r.ReleaseDate)
	return err == nil && day.After(now().UTC())
}

// pinCovers reports whether an Airflow release satisfies a pin that may name
// fewer segments than the release does.
func pinCovers(pin, release string) bool {
	pin = strings.TrimSpace(pin)
	return pin != "" && (release == pin || strings.HasPrefix(release, pin+"."))
}

// seriesOf is an Airflow version's major.minor: "3.3" from "3.3.2" or "3.3".
// Empty when the version names no minor.
func seriesOf(version string) string {
	major, rest, ok := strings.Cut(version, ".")
	if !ok || major == "" {
		return ""
	}
	minor, _, _ := strings.Cut(rest, ".")
	if minor == "" {
		return ""
	}
	return major + "." + minor
}

func majorOf(version string) string {
	major, _, _ := strings.Cut(version, ".")
	return major
}
