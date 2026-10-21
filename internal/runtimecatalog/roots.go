package runtimecatalog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

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
// Until a build has that file, uv works them out from its freeze: one
// `uv pip compile` of every candidate, held to the freeze, whose annotations
// say which of them another one pulls in.
//
// Each is then listed only for the platforms an Astro project locks for where
// uv installs it, and everything it pulls in, from wheels alone: the image
// builds a package from source when it has to, with libraries a laptop may
// lack (mysqlclient has no wheel but Windows'). uv decides that too, a
// `uv pip compile --only-binary` per package and platform, so its rules for
// tags, markers and pure-Python wheels are the ones applied. What Airflow
// pulls in is left to build, since the project installs Airflow anyway.
//
// Left out, either way: Airflow itself, which the project lists already; pip;
// and Astronomer's platform packages (astronomer-* but astronomer-providers:
// the executor, logging, the FAB security manager, runtime extensions), which
// plug into Airflow rather than into a DAG.
//
// Every answer is of one build, Python and platform, which never change, so
// it is kept under the cache root without an expiry: a resolution uv says has
// no answer included. A failure to ask (the network, uv itself) is not kept,
// and is remembered for rootsFailedTTL so a blocked network costs one wait.

// rootsBaseURL is where the published lists are served. A variable so a test
// can point it at a local server.
var rootsBaseURL = "https://cdn.astronomer.io/runtime-roots"

// compileFunc runs `uv pip compile` from dir: input on stdin, args after it.
type compileFunc func(ctx context.Context, dir string, input, args []string) (string, error)

// newCompiler makes the compileFunc one RuntimeRoots uses for all its runs: one
// uv client (uv.Client.PipCompileOut), with the macOS target every resolve for
// Apple silicon needs. A variable so a test answers without uv.
var newCompiler = func(ctx context.Context) (compileFunc, error) {
	root := cacheDir("")
	if root == "" {
		return nil, errors.New("there is no cache directory for uv")
	}
	client, err := uv.New(ctx, uv.Options{CacheDir: filepath.Join(root, "uv")})
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, dir string, input, args []string) (string, error) {
		return client.PipCompileOut(ctx, dir, input, args, []string{macOSTarget})
	}, nil
}

const (
	// rootsCacheDir keeps the published lists, named as the CDN path is, and
	// the 404s for them; rootsComputedDir the lists worked out here;
	// rootsChecksDir what uv said of each package on each platform;
	// rootsFailedDir a list that could not be had, for rootsFailedTTL.
	rootsCacheDir    = "runtime-roots"
	rootsComputedDir = "runtime-roots-computed"
	rootsChecksDir   = "runtime-roots-checks"
	rootsFailedDir   = "runtime-roots-failed"
	// rootsWorkDir holds a directory per call, for the constraints file.
	rootsWorkDir = "runtime-roots-work"
	// rootsBudget bounds one RuntimeRoots.
	rootsBudget = 3 * time.Minute
	// rootsFailedTTL is how long a list that could not be had stays so.
	rootsFailedTTL = time.Hour
	// rootsConcurrency bounds the uv runs at once.
	rootsConcurrency = 8
)

// rootsPlatforms are the platforms an Astro project locks for
// (scaffold.AstroEnvironments, Linux on both machines), as uv names them for
// --python-platform, with the marker that limits a package to each. Linux is
// glibc 2.28, the oldest the runtime images run on.
var rootsPlatforms = []struct {
	uv, name, marker string
}{
	{"x86_64-manylinux_2_28", "Linux x86_64", "sys_platform == 'linux' and platform_machine == 'x86_64'"},
	{"aarch64-manylinux_2_28", "Linux aarch64", "sys_platform == 'linux' and platform_machine == 'aarch64'"},
	{"aarch64-apple-darwin", "macOS arm64", "sys_platform == 'darwin' and platform_machine == 'arm64'"},
}

// macOSTarget is the macOS a resolve for aarch64-apple-darwin targets. uv's
// default for it is older than the wheels many packages publish for Apple
// silicon now (psycopg2-binary's are macosx_14_0), which would read as no
// wheel; macOS 14 is the oldest an Apple-silicon Mac most likely runs.
const macOSTarget = "MACOSX_DEPLOYMENT_TARGET=14.0"

// Roots are what RuntimeRoots found: the entries the astro-runtime group
// lists, sorted, a package limited to some platforms by a marker; and the
// top-level packages left out, with why.
type Roots struct {
	Names   []string
	LeftOut []LeftOut
}

// LeftOut is one of an image's top-level packages the astro-runtime group
// leaves out, and why, in uv's words.
type LeftOut struct {
	Name, Why string
}

// RuntimeRoots returns the top-level packages of runtime build's image, as the
// astro-runtime group lists them for a project that runs python. freeze is the
// package list RuntimePins read for the build.
//
// A list that could not be had is an error. A failure of the whole
// computation, rootsBudget running out included, is remembered for
// rootsFailedTTL, so a start that cannot reach the network waits once an hour,
// not every time; only the caller's own ctx ending is not.
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
	budget, cancel := context.WithTimeout(ctx, rootsBudget)
	defer cancel()
	out, err := runtimeRoots(budget, build, python, freeze)
	switch {
	case err == nil:
		if failed != "" {
			_ = os.Remove(filepath.Join(failed, name)) //nolint:errcheck // a stale note expires on its own
		}
	case ctx.Err() == nil && failed != "" && os.MkdirAll(failed, freezeDirPerm) == nil:
		_ = fsatomic.WriteFile(filepath.Join(failed, name), []byte(err.Error()+"\n"), freezeFilePerm) //nolint:errcheck // a note that cannot be written only means trying again sooner
	}
	return out, err
}

func runtimeRoots(ctx context.Context, build, python string, freeze []string) (Roots, error) {
	root := cacheDir("")
	if root == "" {
		return Roots{}, errors.New("there is no cache directory to work in")
	}
	parent := filepath.Join(root, rootsWorkDir)
	if err := os.MkdirAll(parent, freezeDirPerm); err != nil {
		return Roots{}, err
	}
	work, err := os.MkdirTemp(parent, build+"-python-"+python+"-")
	if err != nil {
		return Roots{}, err
	}
	defer func() { _ = os.RemoveAll(work) }() //nolint:errcheck // a work directory left behind is only disk
	if err := os.WriteFile(filepath.Join(work, "constraints.txt"), []byte(strings.Join(freeze, "\n")+"\n"), freezeFilePerm); err != nil {
		return Roots{}, err
	}
	r := &rootsRun{work: work, python: python, key: build + "-python-" + python, freeze: freeze}
	roots, err := r.roots(ctx, freezeName(build, python))
	if err != nil {
		return Roots{}, fmt.Errorf("working out what Astro Runtime %s installs: %w", build, err)
	}
	return r.withPlatforms(ctx, roots)
}

// rootsRun is one RuntimeRoots: its work directory, the Python, the cache key
// of its build and Python, the build's freeze for that Python, and its uv,
// made the first time an answer is not kept already.
type rootsRun struct {
	work, python, key string
	freeze            []string
	once              sync.Once
	compiler          compileFunc
	compilerErr       error
}

// compile runs `uv pip compile` through the run's one uv client.
func (r *rootsRun) compile(ctx context.Context, input, args []string) (string, error) {
	r.once.Do(func() { r.compiler, r.compilerErr = newCompiler(ctx) })
	if r.compilerErr != nil {
		return "", r.compilerErr
	}
	return r.compiler(ctx, r.work, input, args)
}

// args are what every `uv pip compile` of r takes: the Python, the freeze as
// constraints, and Astronomer's index for its builds of Airflow, which uv
// reads before the default one.
func (r *rootsRun) args(platform string) []string {
	return []string{
		"--python-version", r.python, "--python-platform", platform,
		"--constraint", "constraints.txt", "--extra-index-url", runtimeversions.PublicIndexURL(),
	}
}

// roots is the published list named name, else the one uv works out, kept.
func (r *rootsRun) roots(ctx context.Context, name string) ([]string, error) {
	if roots, ok := publishedRoots(ctx, cacheDir(rootsCacheDir), name); ok {
		return roots, nil
	}
	computed := cacheDir(rootsComputedDir)
	if data, _ := cachedFreeze(computed, name); data != nil { //nolint:errcheck // a computed list has no 404 to remember
		return rootsOf(data), nil
	}
	candidates, airflow := rootsCandidates(r.freeze)
	if len(candidates) == 0 {
		return nil, errors.New("its freeze lists no package to resolve")
	}
	out, err := r.compile(ctx, slices.Concat(candidates, airflow),
		append(r.args(rootsPlatforms[0].uv), "--annotation-style", "line"))
	if err != nil {
		return nil, err
	}
	roots := rootsFromCompile(out, candidates, airflow)
	if len(roots) == 0 {
		return nil, errors.New("uv's resolution left no top-level package")
	}
	if computed != "" && os.MkdirAll(computed, freezeDirPerm) == nil {
		_ = fsatomic.WriteFile(filepath.Join(computed, name), []byte(strings.Join(roots, "\n")+"\n"), freezeFilePerm) //nolint:errcheck // a cache that cannot be written is not a failure
	}
	return roots, nil
}

// rootsCandidates are the freeze's packages the roots are chosen from
// (rootCandidate, less any other +astro build, which only Astronomer's index
// serves), and the Airflow distributions it has.
func rootsCandidates(freeze []string) (candidates, airflow []string) {
	dists := runtimeversions.AirflowDists()
	for _, pin := range freeze {
		name := manifest.DistName(pin)
		switch {
		case slices.Contains(dists, name):
			airflow = append(airflow, name)
		case rootCandidate(name) && !strings.Contains(pin, "+astro."):
			candidates = append(candidates, name)
		}
	}
	slices.Sort(candidates)
	return slices.Compact(candidates), airflow
}

// rootsFromCompile reads the candidates no other one, and no Airflow
// distribution, pulls in from a `uv pip compile --annotation-style line`: a
// line is "name==version  # via a, b, -c constraints.txt".
func rootsFromCompile(out string, candidates, airflow []string) []string {
	pulledIn := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		req, via, ok := strings.Cut(line, "# via ")
		if !ok {
			continue
		}
		name := manifest.DistName(strings.TrimSpace(req))
		for _, parent := range strings.Split(via, ",") {
			parent = strings.TrimSpace(parent)
			if p := manifest.DistName(parent); !strings.HasPrefix(parent, "-") && p != name &&
				(slices.Contains(candidates, p) || slices.Contains(airflow, p)) {
				pulledIn[name] = true
			}
		}
	}
	var roots []string
	for _, c := range candidates {
		if !pulledIn[c] {
			roots = append(roots, c)
		}
	}
	return roots
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
// names, into the names RuntimeRoots starts from.
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

// withPlatforms is roots as the group lists them: each for the platforms uv
// installs it on from wheels alone, by a marker when that is not all of them;
// left out, with uv's reason, when it is none.
func (r *rootsRun) withPlatforms(ctx context.Context, roots []string) (Roots, error) {
	// What Airflow pulls in on each platform may build from source: the
	// project installs it anyway.
	exempt := make([][]string, len(rootsPlatforms))
	for i, p := range rootsPlatforms {
		var err error
		if exempt[i], err = r.airflowPulls(ctx, p.uv); err != nil {
			return Roots{}, err
		}
	}
	type answer struct {
		ok  bool
		why string
		err error
	}
	answers := make([][]answer, len(roots))
	sem := make(chan struct{}, rootsConcurrency)
	var wg sync.WaitGroup
	for i, root := range roots {
		answers[i] = make([]answer, len(rootsPlatforms))
		for j, p := range rootsPlatforms {
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ok, why, err := r.installsFromWheels(ctx, root, p.uv, exempt[j])
				answers[i][j] = answer{ok, why, err}
			}()
		}
	}
	wg.Wait()
	var out Roots
	for i, root := range roots {
		var markers, missing []string
		why := ""
		for j, a := range answers[i] {
			switch {
			case a.err != nil:
				return Roots{}, fmt.Errorf("asking uv whether %s installs from wheels on %s: %w", root, rootsPlatforms[j].name, a.err)
			case a.ok:
				markers = append(markers, rootsPlatforms[j].marker)
			default:
				missing = append(missing, rootsPlatforms[j].name)
				if why == "" {
					why = a.why
				}
			}
		}
		switch {
		case len(missing) == 0:
			out.Names = append(out.Names, root)
		case len(markers) == 0:
			out.LeftOut = append(out.LeftOut, LeftOut{Name: root, Why: why + " on " + strings.Join(missing, ", ") + " (Python " + r.python + ")"})
		case len(markers) == 1:
			out.Names = append(out.Names, root+"; "+markers[0])
		default:
			out.Names = append(out.Names, root+"; ("+strings.Join(markers, ") or (")+")")
		}
	}
	return out, nil
}

// airflowPulls is what the freeze's Airflow pulls in on platform, kept.
func (r *rootsRun) airflowPulls(ctx context.Context, platform string) ([]string, error) {
	path := r.checkPath("airflow@" + platform)
	if data, err := readAnswer(path); err == nil {
		return strings.Fields(data), nil
	}
	_, airflow := rootsCandidates(r.freeze)
	if len(airflow) == 0 {
		return nil, nil
	}
	out, err := r.compile(ctx, airflow, r.args(platform))
	if err != nil {
		return nil, fmt.Errorf("resolving Airflow for %s: %w", platform, err)
	}
	var names []string
	for _, line := range strings.Split(out, "\n") {
		if name := manifest.DistName(strings.TrimSpace(line)); name != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			names = append(names, name)
		}
	}
	keep(path, strings.Join(names, " "), 0)
	return names, nil
}

// missingWheelRes read the package uv names when --only-binary leaves it no
// wheel to install, as uv 0.11 says it: its hint, "Wheels are required for
// `X` because building from source is disabled", and the solver's own "X==v
// has no usable wheels" should the hint's wording change. The backtick uv puts
// around the name is \x60 here.
var missingWheelRes = []*regexp.Regexp{
	regexp.MustCompile(`Wheels are required for \x60([^\x60]+)\x60`),
	regexp.MustCompile(`([A-Za-z0-9][A-Za-z0-9._-]*)==\S+ has no usable wheels`),
}

// missingWheel is the package uv's stderr names for want of a wheel, or "".
func missingWheel(stderr string) string {
	for _, re := range missingWheelRes {
		if m := re.FindStringSubmatch(stderr); m != nil {
			return m[1]
		}
	}
	return ""
}

// installsFromWheels reports whether uv installs root on platform, with all it
// pulls in, from wheels alone, where exempt (what Airflow pulls in) may build:
// every other package of the freeze is held to wheels. why is uv's reason
// when it does not. A failure to ask is an error.
//
// A package uv names for want of a wheel that publishes none at all, for any
// Python or platform (sourceOnly), is taken for pure Python, which builds
// anywhere: it is let build, and the root asked again, up to rootsRetries
// times. One that has wheels somewhere (mysqlclient's are Windows') is native,
// and the root stays out.
//
// The answer is kept for good when it cannot change: it installs, or uv says
// a wheel it needs does not exist. A wheel that exists for an older Python but
// not this one may yet be built, so that answer is kept for wheelsLaterTTL;
// any other no answer from uv, for answerTTL.
func (r *rootsRun) installsFromWheels(ctx context.Context, root, platform string, exempt []string) (ok bool, why string, err error) {
	path := r.checkPath(root + "@" + platform)
	if answer, err := readAnswer(path); err == nil {
		return answer == "ok", strings.TrimPrefix(answer, "no: "), nil
	}
	builds := slices.Clone(exempt)
	ttl := time.Duration(0)
	for range rootsRetries {
		args := r.args(platform)
		for _, pin := range r.freeze {
			if name := manifest.DistName(pin); !slices.Contains(builds, name) {
				args = append(args, "--only-binary", name)
			}
		}
		_, err = r.compile(ctx, []string{root}, args)
		var resolution *uv.ResolutionError
		switch {
		case err == nil:
			keep(path, "ok", 0)
			return true, "", nil
		case !errors.As(err, &resolution):
			return false, "", err
		}
		missing := missingWheel(resolution.Stderr)
		if missing == "" {
			why, ttl = "uv cannot install it from wheels", answerTTL
			break
		}
		named := manifest.DistName(missing)
		if named == root {
			why = "it has no wheel"
		} else {
			why = "it pulls in " + named + ", which has no wheel"
		}
		source, later, err := r.sourceOnly(ctx, named)
		if err != nil {
			return false, "", err
		}
		if later {
			ttl = wheelsLaterTTL
		}
		if !source || slices.Contains(builds, named) {
			break
		}
		builds = append(builds, named)
	}
	keep(path, "no: "+why, ttl)
	return false, why, nil
}

const (
	// rootsRetries bounds the packages installsFromWheels lets build for one
	// root on one platform.
	rootsRetries = 5
	// answerTTL keeps a no answer uv gave for another reason than a missing
	// wheel; wheelsLaterTTL one for a wheel only older Pythons have.
	answerTTL      = time.Hour
	wheelsLaterTTL = 7 * 24 * time.Hour
)

// sourcePlatforms are where sourceOnly looks for a wheel: every platform a
// project locks for, and Windows, where native packages with no wheel for
// Linux or macOS (mysqlclient) still publish one.
var sourcePlatforms = []string{"x86_64-pc-windows-msvc", "x86_64-manylinux_2_28", "aarch64-apple-darwin"}

// sourcePythons are the Pythons sourceOnly asks about besides the project's:
// a native package publishes wheels for the Pythons it has been built for,
// which a new Python is not yet among.
var sourcePythons = []string{"3.12", "3.11"}

// sourceOnly reports whether dist, at the freeze's version, publishes no wheel
// at all: uv installs it from wheels on none of sourcePlatforms, for the
// project's Python or any of sourcePythons. later reports one with wheels for
// another Python only, not this one: native, and not built for this Python
// yet, which can change. Kept, the first for good and the second for
// wheelsLaterTTL.
func (r *rootsRun) sourceOnly(ctx context.Context, dist string) (source, later bool, err error) {
	path := r.checkPath("source-only@" + dist)
	if answer, err := readAnswer(path); err == nil {
		return answer == "yes", answer == "later", nil
	}
	pythons := slices.Compact(append([]string{r.python}, sourcePythons...))
	for _, python := range pythons {
		for _, platform := range sourcePlatforms {
			args := append(r.args(platform), "--only-binary", dist)
			args[1] = python
			_, err := r.compile(ctx, []string{dist}, args)
			var resolution *uv.ResolutionError
			switch {
			case err == nil && python == r.python:
				keep(path, "no", 0)
				return false, false, nil
			case err == nil:
				keep(path, "later", wheelsLaterTTL)
				return false, true, nil
			case !errors.As(err, &resolution):
				return false, false, err
			}
		}
	}
	keep(path, "yes", 0)
	return true, false, nil
}

// checkPath is where an answer of r named name is kept, "" for no cache.
func (r *rootsRun) checkPath(name string) string {
	dir := cacheDir(rootsChecksDir)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, r.key, name)
}

// readAnswer reads a kept answer, unless it has expired; with no cache (path
// "") there is none.
func readAnswer(path string) (string, error) {
	if path == "" {
		return "", os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	answer, expires, _ := strings.Cut(strings.TrimSpace(string(data)), "\nexpires ")
	if expires != "" {
		at, err := time.Parse(time.RFC3339, strings.TrimSpace(expires))
		if err != nil || time.Now().After(at) {
			return "", os.ErrNotExist
		}
	}
	return answer, nil
}

// keep writes an answer to path, atomically, unless there is no cache: for
// good, or for ttl when it is not 0. A cache that cannot be written is not a
// failure.
func keep(path, answer string, ttl time.Duration) {
	if path == "" || os.MkdirAll(filepath.Dir(path), freezeDirPerm) != nil {
		return
	}
	if ttl > 0 {
		answer += "\nexpires " + time.Now().Add(ttl).UTC().Format(time.RFC3339)
	}
	_ = fsatomic.WriteFile(path, []byte(answer+"\n"), freezeFilePerm) //nolint:errcheck // see above
}
