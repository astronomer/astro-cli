package runtimecatalog

import (
	"context"
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
	// the 404s for them; rootsComputedDir keeps the lists worked out here.
	rootsCacheDir    = "runtime-roots"
	rootsComputedDir = "runtime-roots-computed"
	// rootsWorkDir is where the scratch project is resolved, removed after.
	rootsWorkDir = "runtime-roots-work"
	// rootsLockTimeout bounds the scratch resolution: about 190 exact pins
	// whose metadata a cold uv cache downloads once.
	rootsLockTimeout = 3 * time.Minute
	// rootsProject names the scratch project, so its own edges are told
	// apart from the image's in the lockfile.
	rootsProject = "astro-runtime-roots"
)

// RuntimeRoots returns the top-level packages of runtime build's image
// running python, as normalized distribution names, sorted, less Airflow,
// pip and Astronomer's own: the published list when there is one, else the
// one worked out from the build's freeze (RuntimePins) and kept.
//
// It says why it has nothing as RuntimePins does: ErrFreezeNotPublished for
// a build with no freeze, ErrPinnedResolution under a UV_EXCLUDE_NEWER cutoff,
// any other error when neither the list nor the freeze could be read or
// resolved.
func RuntimeRoots(ctx context.Context, build, python string) ([]string, error) {
	if strings.TrimSpace(os.Getenv("UV_EXCLUDE_NEWER")) != "" {
		return nil, ErrPinnedResolution
	}
	if !freezeNamePart.MatchString(build) || !freezeNamePart.MatchString(python) {
		return nil, fmt.Errorf("%w: build %q, Python %q", ErrFreezeNotPublished, build, python)
	}
	// The freeze says which version of each the image has, which the
	// platforms are judged by, and is what a list is worked out from.
	pins, err := RuntimePins(ctx, build, python)
	if err != nil {
		return nil, err
	}
	roots, err := rootsFor(ctx, build, python, pins)
	if err != nil {
		return nil, err
	}
	return withPlatforms(ctx, roots, pins, python)
}

// rootsFor is the published list of build and python, else the one worked out
// from pins, kept once worked out.
func rootsFor(ctx context.Context, build, python string, pins []string) ([]string, error) {
	name := freezeName(build, python)
	if roots, ok := publishedRoots(ctx, cacheDir(rootsCacheDir), name); ok {
		return roots, nil
	}
	computed := cacheDir(rootsComputedDir)
	if data, _ := cachedFreeze(computed, name); data != nil { //nolint:errcheck // a computed list has no 404 to remember
		return rootsOf(data), nil
	}
	roots, err := computeRoots(ctx, python, pins)
	if err != nil {
		return nil, fmt.Errorf("working out what Astro Runtime %s installs: %w", build, err)
	}
	if computed != "" && os.MkdirAll(computed, freezeDirPerm) == nil {
		_ = fsatomic.WriteFile(filepath.Join(computed, name), []byte(strings.Join(roots, "\n")+"\n"), freezeFilePerm) //nolint:errcheck // a cache that cannot be written is not a failure
	}
	return roots, nil
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

// airflowDists are the distributions the project lists itself, which are no
// part of the list.
var airflowDists = []string{runtimeversions.DistAirflow, runtimeversions.DistCore, runtimeversions.DistTaskSDK}

// dagFacingAstronomer is the one astronomer-* package a DAG imports:
// astronomer-providers' operators and sensors. The others plug into Airflow
// itself (the executor, logging, the FAB security manager, the version check,
// runtime extensions), which locally would change how Airflow behaves.
const dagFacingAstronomer = "astronomer-providers"

// rootCandidate reports whether dist, normalized, can be one of the
// RuntimeRoots: not Airflow, pip or one of Astronomer's platform packages.
func rootCandidate(dist string) bool {
	return dist != "" && dist != "pip" && !slices.Contains(airflowDists, dist) &&
		(!strings.HasPrefix(dist, "astronomer-") || dist == dagFacingAstronomer)
}

// computeRoots works out the top-level packages of a freeze: it resolves a
// scratch project that depends on every candidate (rootCandidate, less any
// other +astro build, which only Astronomer's index serves) and holds each to
// the freeze's version, then reads which of them no package in the resolution
// requires, through a dependency or one of its extras.
func computeRoots(ctx context.Context, python string, pins []string) ([]string, error) {
	root := cacheDir("")
	if root == "" {
		return nil, errors.New("there is no cache directory to resolve in")
	}
	work := filepath.Join(root, rootsWorkDir, "python-"+python)
	if err := os.RemoveAll(work); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(work, freezeDirPerm); err != nil {
		return nil, err
	}
	defer os.RemoveAll(work)
	candidates, deps := rootsProjectDeps(pins)
	if len(candidates) == 0 {
		return nil, errors.New("its freeze lists no package to resolve")
	}
	if err := os.WriteFile(filepath.Join(work, manifest.Marker), rootsPyproject(python, deps, pins), freezeFilePerm); err != nil {
		return nil, err
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv"), NoConfig: true})
	if err != nil {
		return nil, err
	}
	lockCtx, cancel := context.WithTimeout(ctx, rootsLockTimeout)
	defer cancel()
	if err := client.Lock(lockCtx, work, uv.Stdio{}); err != nil {
		return nil, err
	}
	lock, err := os.ReadFile(filepath.Join(work, "uv.lock"))
	if err != nil {
		return nil, err
	}
	return rootsFromLock(lock, candidates)
}

// rootsProjectDeps splits a freeze's pins into the candidates the roots are
// chosen from and the scratch project's dependencies: the candidates and the
// Airflow distributions the freeze has, whose sources name Astronomer's index
// and apply to direct dependencies alone.
func rootsProjectDeps(pins []string) (candidates, deps []string) {
	for _, pin := range pins {
		name := manifest.DistName(pin)
		switch {
		case slices.Contains(airflowDists, name):
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
	for _, d := range airflowDists {
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

// rootsFromLock reads a scratch project's uv.lock and returns the candidates
// no other package in it requires: through a dependency, or through an extra
// a dependency names, which installs that extra's packages too. The scratch
// project's own edges, to every candidate, are not counted.
func rootsFromLock(data []byte, candidates []string) ([]string, error) {
	var lock struct {
		Package []lockPackage `toml:"package"`
	}
	if err := toml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("reading the scratch project's uv.lock: %w", err)
	}
	optional := map[string]map[string][]lockDependency{}
	for _, p := range lock.Package {
		optional[manifest.DistName(p.Name)] = p.Optional
	}
	required := map[string]bool{}
	// An extra can name the package's own extras, so each is followed once.
	followed := map[string]bool{}
	var require func(deps []lockDependency)
	require = func(deps []lockDependency) {
		for _, d := range deps {
			name := manifest.DistName(d.Name)
			required[name] = true
			for _, extra := range d.Extra {
				if key := name + "[" + extra + "]"; !followed[key] {
					followed[key] = true
					require(optional[name][extra])
				}
			}
		}
	}
	for _, p := range lock.Package {
		if manifest.DistName(p.Name) != rootsProject {
			require(p.Dependencies)
		}
	}
	var roots []string
	for _, c := range candidates {
		if !required[c] {
			roots = append(roots, c)
		}
	}
	if len(roots) == 0 {
		return nil, errors.New("the resolution left no top-level package")
	}
	return roots, nil
}
