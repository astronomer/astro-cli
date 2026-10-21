package runtimecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// The packages an Astro Runtime image installs on purpose: its top-level
// distributions, the ones nothing else it has installed requires (`pip list
// --not-required`). The rest of its freeze is what they pull in. astro lists
// them in the project's astro-runtime dependency group (scaffold.RuntimeGroup),
// so standalone mode has what the image has: a DAG that imports pandas or a
// provider the image preinstalls imports it locally too.
//
// The runtime team is asked to publish them beside the freeze
// (astronomer/astro-runtime#3645), at
//
//	https://cdn.astronomer.io/runtime-roots/runtime-<build>-python-<python>.txt
//
// Until a build has that file they are worked out here from its freeze:
// resolved once in a scratch project, and the ones no other package of the
// image requires taken from the lockfile's graph. Either answer is of one
// build and never changes, so it is kept under the cache root without an
// expiry.
//
// Left out, either way: Airflow itself, which the project lists already; pip;
// and Astronomer's platform packages (astronomer-* but astronomer-providers:
// the executor, logging, the FAB security manager, runtime extensions), which
// plug into Airflow rather than into a DAG, and some of which come from an
// index the project reads for Airflow alone.

// rootsBaseURL is where the published lists are served. A variable so a test
// can point it at a local server.
var rootsBaseURL = "https://cdn.astronomer.io/runtime-roots"

const (
	// rootsCacheDir keeps the published lists, named as the CDN path is, and
	// the 404s for them; rootsComputedDir keeps the lists worked out here;
	// rootsFailedDir remembers a list that could not be had, for
	// rootsFailedTTL.
	rootsCacheDir    = "runtime-roots"
	rootsComputedDir = "runtime-roots-computed"
	rootsFailedDir   = "runtime-roots-failed"
	// rootsWorkDir holds the scratch projects, each in a directory of its
	// own, removed after.
	rootsWorkDir = "runtime-roots-work"
	// rootsBudget bounds one RuntimeRoots: the published list, a scratch
	// resolution of about 190 exact pins whose metadata a cold uv cache
	// downloads once, and the index pages the platforms are read from.
	rootsBudget = 3 * time.Minute
	// rootsFailedTTL is how long a list that could not be had stays so: long
	// enough that a blocked network does not run a resolution on every
	// start, short enough to try again within the day.
	rootsFailedTTL = time.Hour
	// rootsProject names the scratch project, so its own edges are told
	// apart from the image's in the lockfile.
	rootsProject = "astro-runtime-roots"
)

// Roots are what RuntimeRoots found: the entries the astro-runtime group
// lists, sorted, a package limited to some platforms by a marker; and the
// top-level packages left out, with why (withPlatforms).
type Roots struct {
	Names   []string
	LeftOut []LeftOut
}

// RuntimeRoots returns the top-level packages of runtime build's image running
// python, whose freeze, the pins RuntimePins read for it, is freeze: the
// published list when there is one, else the one worked out from freeze and
// kept, less Airflow, pip and Astronomer's platform packages, each limited to
// the platforms it has a wheel for.
//
// A list that could not be had (no network, uv failing) is an error, and the
// error is remembered for rootsFailedTTL, so a start that cannot reach the
// network waits once, not every time.
func RuntimeRoots(ctx context.Context, build, python string, freeze []string) (Roots, error) {
	if !freezeNamePart.MatchString(build) || !freezeNamePart.MatchString(python) {
		return Roots{}, fmt.Errorf("%w: build %q, Python %q", ErrFreezeNotPublished, build, python)
	}
	name := freezeName(build, python)
	failed := cacheDir(rootsFailedDir)
	if failed != "" {
		path := filepath.Join(failed, name)
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) < rootsFailedTTL {
			data, _ := os.ReadFile(path) //nolint:errcheck // an unreadable note says only that it failed
			return Roots{}, fmt.Errorf("working out what Astro Runtime %s installs failed less than an hour ago, and is not tried again until then: %s", build, strings.TrimSpace(string(data)))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, rootsBudget)
	defer cancel()
	roots, closures, err := rootsFor(ctx, build, python, freeze)
	var out Roots
	if err == nil {
		out, err = withPlatforms(ctx, roots, closures, freeze, python)
	}
	if err != nil {
		if failed != "" && os.MkdirAll(failed, freezeDirPerm) == nil {
			_ = fsatomic.WriteFile(filepath.Join(failed, name), []byte(err.Error()+"\n"), freezeFilePerm) //nolint:errcheck // a note that cannot be written only means trying again sooner
		}
		return Roots{}, err
	}
	if failed != "" {
		_ = os.Remove(filepath.Join(failed, name)) //nolint:errcheck // a stale note expires on its own
	}
	return out, nil
}

// rootGraph is what the scratch resolution says of an image's top-level
// packages: which they are, and what each pulls in (its closure, the package
// itself left out), as kept under rootsComputedDir.
type rootGraph struct {
	Roots    []string            `json:"roots"`
	Closures map[string][]string `json:"closures"`
}

// rootsFor is the published list of build and python, else the one worked out
// from pins, and what each of them pulls in, which only the scratch
// resolution can say: it is worked out once per build and Python and kept,
// a published list or not.
func rootsFor(ctx context.Context, build, python string, pins []string) (roots []string, closures map[string][]string, err error) {
	name := freezeName(build, python)
	graph, err := computedGraph(ctx, build, python, pins)
	if err != nil {
		return nil, nil, err
	}
	if published, ok := publishedRoots(ctx, cacheDir(rootsCacheDir), name); ok {
		return published, graph.Closures, nil
	}
	return graph.Roots, graph.Closures, nil
}

// computedGraph is the rootGraph of build and python, from the cache, else
// worked out from pins and kept.
func computedGraph(ctx context.Context, build, python string, pins []string) (rootGraph, error) {
	dir := cacheDir(rootsComputedDir)
	path := filepath.Join(dir, freezeName(build, python)+".json")
	var graph rootGraph
	if dir != "" {
		if data, err := os.ReadFile(path); err == nil && json.Unmarshal(data, &graph) == nil && len(graph.Roots) > 0 {
			return graph, nil
		}
	}
	graph, err := computeRoots(ctx, build, python, pins)
	if err != nil {
		return rootGraph{}, fmt.Errorf("working out what Astro Runtime %s installs: %w", build, err)
	}
	if data, err := json.Marshal(graph); err == nil && dir != "" && os.MkdirAll(dir, freezeDirPerm) == nil {
		_ = fsatomic.WriteFile(path, data, freezeFilePerm) //nolint:errcheck // a cache that cannot be written is not a failure
	}
	return graph, nil
}

// publishedRoots is the published list for name, from the cache under dir or
// the CDN, and whether there was one. A 404, remembered as the freeze's are,
// or any failure to read it, is no list.
func publishedRoots(ctx context.Context, dir, name string) ([]string, bool) {
	data, err := cachedFreeze(dir, name)
	if err != nil {
		return nil, false
	}
	if data == nil {
		fetchCtx, cancel := context.WithTimeout(ctx, freezeTimeout)
		defer cancel()
		if data, err = fetchAndKeep(fetchCtx, rootsBaseURL, dir, name); err != nil {
			return nil, false
		}
	}
	roots := rootsOf(data)
	return roots, len(roots) > 0
}

// rootsOf reads a list of top-level packages, `pip freeze` lines or bare
// names, into the names RuntimeRoots returns.
func rootsOf(data []byte) []string {
	dists, err := parseFreeze(data)
	if err != nil {
		return nil
	}
	var roots []string
	for _, d := range dists {
		if rootCandidate(d) && !slices.Contains(roots, d) {
			roots = append(roots, d)
		}
	}
	slices.Sort(roots)
	return roots
}

// dagFacingAstronomer is the one astronomer-* package a DAG imports:
// astronomer-providers' operators and sensors. The others plug into Airflow
// itself (the executor, logging, the FAB security manager, the version check,
// runtime extensions), which locally would change how Airflow behaves.
const dagFacingAstronomer = "astronomer-providers"

// rootCandidate reports whether dist, normalized, can be one of the
// RuntimeRoots: not Airflow, pip or one of Astronomer's platform packages.
func rootCandidate(dist string) bool {
	return dist != "" && dist != "pip" && !slices.Contains(runtimeversions.AirflowDists(), dist) &&
		(!strings.HasPrefix(dist, "astronomer-") || dist == dagFacingAstronomer)
}

// computeRoots works out the top-level packages of a freeze: it resolves a
// scratch project that depends on every candidate (rootCandidate, less any
// other +astro build, which only Astronomer's index serves) and holds each to
// the freeze's version, then reads which of them no package in the resolution
// requires, through a dependency or one of its extras.
//
// The project sits in a directory made for this call alone, so two at once
// never read each other's lockfile. uv reads the user's own configuration, so
// a mirror they resolve through serves this too.
func computeRoots(ctx context.Context, build, python string, pins []string) (rootGraph, error) {
	root := cacheDir("")
	if root == "" {
		return rootGraph{}, errors.New("there is no cache directory to resolve in")
	}
	candidates, deps := rootsProjectDeps(pins)
	if len(candidates) == 0 {
		return rootGraph{}, errors.New("its freeze lists no package to resolve")
	}
	parent := filepath.Join(root, rootsWorkDir)
	if err := os.MkdirAll(parent, freezeDirPerm); err != nil {
		return rootGraph{}, err
	}
	work, err := os.MkdirTemp(parent, build+"-python-"+python+"-")
	if err != nil {
		return rootGraph{}, err
	}
	defer func() { _ = os.RemoveAll(work) }() //nolint:errcheck // a scratch directory left behind is only disk
	if err := os.WriteFile(filepath.Join(work, manifest.Marker), rootsPyproject(python, deps, pins), freezeFilePerm); err != nil {
		return rootGraph{}, err
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
	if err != nil {
		return rootGraph{}, err
	}
	if err := client.Lock(ctx, work, uv.Stdio{}); err != nil {
		return rootGraph{}, err
	}
	lock, err := os.ReadFile(filepath.Join(work, "uv.lock"))
	if err != nil {
		return rootGraph{}, err
	}
	return rootsFromLock(lock, candidates)
}

// rootsProjectDeps splits a freeze's pins into the candidates the roots are
// chosen from and the scratch project's dependencies: the candidates and the
// Airflow distributions the freeze has, whose sources name Astronomer's index
// and apply to direct dependencies alone.
func rootsProjectDeps(pins []string) (candidates, deps []string) {
	airflow := runtimeversions.AirflowDists()
	for _, pin := range pins {
		name := manifest.DistName(pin)
		switch {
		case slices.Contains(airflow, name):
			deps = append(deps, name)
		case rootCandidate(name) && !strings.Contains(pin, "+astro."):
			candidates = append(candidates, name)
		}
	}
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)
	return candidates, slices.Concat(candidates, deps)
}

// rootsPyproject is the scratch project's pyproject.toml: deps, held to pins
// on the image's Python and platform, Airflow from Astronomer's index.
func rootsPyproject(python string, deps, pins []string) []byte {
	var b strings.Builder
	list := func(key string, values []string) {
		b.WriteString(key + " = [\n")
		for _, v := range values {
			b.WriteString("    " + strconv.Quote(v) + ",\n")
		}
		b.WriteString("]\n")
	}
	b.WriteString("[project]\nname = " + strconv.Quote(rootsProject) + "\nversion = \"0\"\n")
	b.WriteString("requires-python = " + strconv.Quote("=="+python+".*") + "\n")
	list("dependencies", deps)
	b.WriteString("\n[tool.uv]\n")
	list("constraint-dependencies", pins)
	list("environments", []string{"sys_platform == 'linux'"})
	b.WriteString("\n[[tool.uv.index]]\nname = \"astronomer\"\nurl = " + strconv.Quote(runtimeversions.PublicIndexURL()) + "\nexplicit = true\n")
	b.WriteString("\n[tool.uv.sources]\n")
	for _, d := range runtimeversions.AirflowDists() {
		if slices.Contains(deps, d) {
			b.WriteString(strconv.Quote(d) + " = { index = \"astronomer\" }\n")
		}
	}
	return []byte(b.String())
}

// lockPackage is the part of a uv.lock [[package]] entry the graph needs.
type lockPackage struct {
	Name         string                      `toml:"name"`
	Dependencies []lockDependency            `toml:"dependencies"`
	Optional     map[string][]lockDependency `toml:"optional-dependencies"`
}

type lockDependency struct {
	Name  string   `toml:"name"`
	Extra []string `toml:"extra"`
}

// rootsFromLock reads a scratch project's uv.lock: the candidates no other
// package in it requires, through a dependency, or through an extra a
// dependency names, which installs that extra's packages too; and what each
// candidate pulls in, the same way, less Airflow and what it pulls in, which
// the project installs anyway. The scratch project's own edges, to every
// candidate, are not counted. The resolution is for Linux, the image's
// platform, so a dependency only another platform needs is not in it.
func rootsFromLock(data []byte, candidates []string) (rootGraph, error) {
	var lock struct {
		Package []lockPackage `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return rootGraph{}, fmt.Errorf("reading the scratch project's uv.lock: %w", err)
	}
	packages := map[string]lockPackage{}
	for _, p := range lock.Package {
		packages[manifest.DistName(p.Name)] = p
	}
	// reach adds to into what deps pull in, following each extra once: an
	// extra can name the package's own extras.
	var reach func(deps []lockDependency, into map[string]bool, followed map[string]bool)
	reach = func(deps []lockDependency, into map[string]bool, followed map[string]bool) {
		for _, d := range deps {
			name := manifest.DistName(d.Name)
			if !into[name] {
				into[name] = true
				reach(packages[name].Dependencies, into, followed)
			}
			for _, extra := range d.Extra {
				if key := name + "[" + extra + "]"; !followed[key] {
					followed[key] = true
					reach(packages[name].Optional[extra], into, followed)
				}
			}
		}
	}
	// require marks what deps name directly, and through the extras they
	// name, which install that extra's packages too.
	required, followed := map[string]bool{}, map[string]bool{}
	var require func(deps []lockDependency)
	require = func(deps []lockDependency) {
		for _, d := range deps {
			name := manifest.DistName(d.Name)
			required[name] = true
			for _, extra := range d.Extra {
				if key := name + "[" + extra + "]"; !followed[key] {
					followed[key] = true
					require(packages[name].Optional[extra])
				}
			}
		}
	}
	for _, p := range lock.Package {
		if manifest.DistName(p.Name) != rootsProject {
			require(p.Dependencies)
		}
	}
	// What Airflow pulls in is installed whatever the group says, since the
	// project lists Airflow: a root's closure is what it adds to that.
	airflow := map[string]bool{}
	for _, d := range runtimeversions.AirflowDists() {
		if _, ok := packages[d]; ok {
			airflow[d] = true
			reach(packages[d].Dependencies, airflow, map[string]bool{})
		}
	}
	graph := rootGraph{Closures: map[string][]string{}}
	for _, c := range candidates {
		if required[c] {
			continue
		}
		graph.Roots = append(graph.Roots, c)
		into := map[string]bool{}
		reach(packages[c].Dependencies, into, map[string]bool{})
		delete(into, c)
		closure := make([]string, 0, len(into))
		for name := range into {
			if !airflow[name] {
				closure = append(closure, name)
			}
		}
		slices.Sort(closure)
		graph.Closures[c] = closure
	}
	if len(graph.Roots) == 0 {
		return rootGraph{}, errors.New("the resolution left no top-level package")
	}
	return graph, nil
}
