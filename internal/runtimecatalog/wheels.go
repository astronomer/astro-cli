package runtimecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/version"
)

// The runtime image installs its packages on Linux, where it can build one
// from source with the system libraries it ships. A laptop often cannot:
// mysqlclient publishes no wheel but Windows', and building it needs MySQL's
// client library. So each of the RuntimeRoots goes into the group only for
// the platforms an Astro project locks for (scaffold.AstroEnvironments: Linux
// on x86_64 and aarch64, macOS on Apple silicon) that its version has a wheel
// for on the image's Python, by a marker; a package with a wheel for none of
// them is left out, and Docker mode still has it. A release with no wheel at
// all is pure Python as far as this can tell, and builds anywhere.

// pypiSimpleURL is the simple index read when the environment names none. A
// variable so a test can point it at a local server.
var pypiSimpleURL = "https://pypi.org/simple"

const (
	// wheelsCacheDir keeps what the index said of one version's wheels on one
	// Python: forever when it covers every platform, which never changes, and
	// for wheelsPartialTTL otherwise, since a missing wheel can be uploaded.
	wheelsCacheDir   = "runtime-wheels"
	wheelsPartialTTL = 7 * 24 * time.Hour
	// wheelsMaxBytes caps one project's index page. A project with many wheels
	// a release has a big one: pydantic-core's JSON page is about 10 MB.
	wheelsMaxBytes = 64 << 20
)

// The platforms an Astro project locks for, as bits of what a version covers.
const (
	linuxX86 = 1 << iota
	linuxArm
	darwinArm

	everyPlatform = linuxX86 | linuxArm | darwinArm
)

// platformMarkers are the markers of each platform, in the order a marker
// lists them.
var platformMarkers = []struct {
	bit    int
	marker string
}{
	{linuxX86, "sys_platform == 'linux' and platform_machine == 'x86_64'"},
	{linuxArm, "sys_platform == 'linux' and platform_machine == 'aarch64'"},
	{darwinArm, "sys_platform == 'darwin' and platform_machine == 'arm64'"},
}

// LeftOut is one of an image's top-level packages the astro-runtime group
// leaves out, and why: it, or a package it pulls in, has no wheel for a
// platform an Astro project runs on.
type LeftOut struct {
	Name, Why string
}

// wheelsConcurrency bounds the index requests in flight at once.
const wheelsConcurrency = 16

// withPlatforms is roots as the group lists them. A root whose closure, the
// packages it pulls in (closures), has one with no wheel for a platform is
// left out, as the image builds that package from source with libraries a
// laptop may lack: providers-mysql pulls in mysqlclient on Linux, which has no
// Linux wheel. A root that is short of a platform by its own wheels alone is
// limited to the ones it has by a marker; with none, it is left out too.
// Versions come from pins; a package pins has none for is taken to install
// anywhere. Any index page that cannot be read fails the whole list, which
// keeps the group as it was rather than one short of a package.
func withPlatforms(ctx context.Context, roots []string, closures map[string][]string, pins []string, python string) (Roots, error) {
	versions := map[string]string{}
	for _, pin := range pins {
		if name, v, ok := strings.Cut(pin, "=="); ok {
			versions[manifest.DistName(name)] = strings.TrimSpace(v)
		}
	}
	var names []string
	for _, root := range roots {
		names = append(names, root)
		names = append(names, closures[root]...)
	}
	slices.Sort(names)
	names = slices.Compact(names)
	where, err := platformsOf(ctx, names, versions, python)
	if err != nil {
		return Roots{}, err
	}
	var out Roots
	for _, root := range roots {
		if dep, missing := shortOf(closures[root], where); dep != "" {
			out.LeftOut = append(out.LeftOut, LeftOut{Name: root, Why: "it pulls in " + dep + ", which has no wheel for " + platformNames(missing)})
			continue
		}
		switch w := where[root]; w {
		case everyPlatform:
			out.Names = append(out.Names, root)
		case 0:
			out.LeftOut = append(out.LeftOut, LeftOut{Name: root, Why: "it has no wheel for " + platformNames(everyPlatform)})
		default:
			out.Names = append(out.Names, root+"; "+platformMarker(w))
		}
	}
	return out, nil
}

// platformsOf is what each of names covers, its version from versions, read
// with at most wheelsConcurrency requests at once.
func platformsOf(ctx context.Context, names []string, versions map[string]string, python string) (map[string]int, error) {
	where := map[string]int{}
	var mu sync.Mutex
	var firstErr error
	sem := make(chan struct{}, wheelsConcurrency)
	var wg sync.WaitGroup
	for _, name := range names {
		v, ok := versions[name]
		if !ok {
			where[name] = everyPlatform
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			w, err := wheelsOf(ctx, name, v, python)
			mu.Lock()
			defer mu.Unlock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("reading which platforms %s has wheels for: %w", name, err)
			}
			where[name] = w
		}()
	}
	wg.Wait()
	return where, firstErr
}

// shortOf is the first of closure, in order, that does not cover every
// platform, and the platforms it misses; "" when all of them do.
func shortOf(closure []string, where map[string]int) (dep string, missing int) {
	for _, d := range closure {
		if w := where[d]; w != everyPlatform {
			return d, everyPlatform &^ w
		}
	}
	return "", 0
}

// platformNames names the platforms in bits, for a message.
func platformNames(bits int) string {
	var names []string
	for _, p := range []struct {
		bit  int
		name string
	}{{linuxX86, "Linux x86_64"}, {linuxArm, "Linux aarch64"}, {darwinArm, "macOS arm64"}} {
		if bits&p.bit != 0 {
			names = append(names, p.name)
		}
	}
	return strings.Join(names, ", ")
}

// platformMarker is the marker that limits a package to the platforms where
// covers, some of them.
func platformMarker(where int) string {
	var parts []string
	for _, p := range platformMarkers {
		if where&p.bit != 0 {
			parts = append(parts, p.marker)
		}
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return "(" + strings.Join(parts, ") or (") + ")"
}

// wheelsOf is what dist's release covers on python: from the cache, else from
// the index page for dist, then kept.
func wheelsOf(ctx context.Context, dist, release, python string) (int, error) {
	dir := cacheDir(wheelsCacheDir)
	name := dist + "==" + release + "-python-" + python
	if dir != "" {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil {
			data, _ := os.ReadFile(path) //nolint:errcheck // an unreadable answer is asked again
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && (n == everyPlatform || time.Since(info.ModTime()) < wheelsPartialTTL) {
				return n, nil
			}
		}
	}
	files, err := indexFiles(ctx, dist)
	if err != nil {
		return 0, err
	}
	where := wheelPlatforms(files, release, python)
	if dir != "" && os.MkdirAll(dir, freezeDirPerm) == nil {
		_ = fsatomic.WriteFile(filepath.Join(dir, name), []byte(strconv.Itoa(where)), freezeFilePerm) //nolint:errcheck // a cache that cannot be written is not a failure
	}
	return where, nil
}

// simpleIndexURL is the simple index the project's packages come from: the
// one uv or pip is pointed at in the environment, a mirror most often, else
// pypiSimpleURL.
func simpleIndexURL() string {
	for _, key := range []string{"UV_DEFAULT_INDEX", "UV_INDEX_URL", "PIP_INDEX_URL"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return strings.TrimSuffix(v, "/")
		}
	}
	return pypiSimpleURL
}

// anchorRe reads a file name out of a simple index's HTML page (PEP 503): the
// text of each link.
var anchorRe = regexp.MustCompile(`(?i)<a\s[^>]*>([^<]+)</a>`)

// indexFiles reads the file names the simple index lists for dist, from its
// JSON page (PEP 691), or its HTML page from an index that serves no JSON.
func indexFiles(ctx context.Context, dist string) ([]string, error) {
	url := simpleIndexURL() + "/" + dist + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.pypi.simple.v1+json, text/html;q=0.1")
	req.Header.Set("User-Agent", "astro-cli/"+version.Current())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading the index page of %s: %s", dist, resp.Status)
	}
	// One byte past the cap, so a page the cap would cut short is refused
	// rather than read as a shorter one.
	body, err := io.ReadAll(io.LimitReader(resp.Body, wheelsMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > wheelsMaxBytes {
		return nil, fmt.Errorf("reading the index page of %s: larger than %d bytes", dist, wheelsMaxBytes)
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "html") {
		var files []string
		for _, m := range anchorRe.FindAllSubmatch(body, -1) {
			files = append(files, strings.TrimSpace(html.UnescapeString(string(m[1]))))
		}
		return files, nil
	}
	var page struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("reading the index page of %s: %w", dist, err)
	}
	files := make([]string, len(page.Files))
	for i, f := range page.Files {
		files[i] = f.Filename
	}
	return files, nil
}

// wheelPlatforms is what the wheels of release among files cover on python
// ("3.14"): linuxX86 or linuxArm for a glibc manylinux wheel of x86_64 or
// aarch64, darwinArm for a macOS wheel of arm64 or universal2, every platform
// for a pure-Python one or for a release with no wheel at all. A wheel's
// Python tag has to be py3 or this CPython, with no ABI, the stable ABI of an
// older CPython, or this CPython's own (not its free-threaded build).
func wheelPlatforms(files []string, release, python string) int {
	minor, err := strconv.Atoi(strings.TrimPrefix(python, "3."))
	if err != nil || !strings.HasPrefix(python, "3.") {
		return 0
	}
	cp := "cp3" + strconv.Itoa(minor)
	where, anyWheel := 0, false
	for _, f := range files {
		parts := strings.Split(strings.TrimSuffix(f, ".whl"), "-")
		if !strings.HasSuffix(f, ".whl") || len(parts) < 5 || parts[1] != release {
			continue
		}
		anyWheel = true
		pyTags, abi, platforms := parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
		if !pythonFits(pyTags, abi, cp, minor) {
			continue
		}
		for _, p := range strings.Split(platforms, ".") {
			switch {
			case p == "any":
				where |= everyPlatform
			case strings.HasPrefix(p, "manylinux") && strings.HasSuffix(p, "_x86_64"):
				where |= linuxX86
			case strings.HasPrefix(p, "manylinux") && strings.HasSuffix(p, "_aarch64"):
				where |= linuxArm
			case strings.HasPrefix(p, "macosx_") && (strings.HasSuffix(p, "_arm64") || strings.HasSuffix(p, "_universal2")):
				where |= darwinArm
			}
		}
	}
	if !anyWheel {
		return everyPlatform
	}
	return where
}

// pythonFits reports whether a wheel's Python and ABI tags install on CPython
// 3.minor (cp).
func pythonFits(pyTags, abi, cp string, minor int) bool {
	for _, t := range strings.Split(pyTags, ".") {
		switch {
		case t == "py3" && abi == "none":
			return true
		case t == cp && (abi == cp || abi == "none" || abi == "abi3"):
			return true
		case strings.HasPrefix(t, "cp3") && abi == "abi3":
			if n, err := strconv.Atoi(strings.TrimPrefix(t, "cp3")); err == nil && n <= minor {
				return true
			}
		}
	}
	return false
}
