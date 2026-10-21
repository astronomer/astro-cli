package runtimecatalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
// client library. So each of the RuntimeRoots goes into the group only for the
// platforms PyPI has a wheel of its version for, on the image's Python: Linux,
// Apple silicon macOS (the platforms an Astro project locks for), or both, by
// a sys_platform marker; a package with neither is left out, and Docker mode
// still has it.

// pypiSimpleURL is PyPI's simple index, read as JSON (PEP 691). A variable so
// a test can point it at a local server.
var pypiSimpleURL = "https://pypi.org/simple"

const (
	// wheelsCacheDir keeps what PyPI said of one version's wheels on one
	// Python, which never changes.
	wheelsCacheDir = "runtime-wheels"
	// wheelsTimeout bounds every request of one RuntimeRoots together.
	wheelsTimeout = 15 * time.Second
	// wheelsMaxBytes caps one project's index page.
	wheelsMaxBytes = 8 << 20
)

// Where a version has a wheel for.
const (
	onLinux = 1 << iota
	onDarwin
)

// withPlatforms is roots as the group lists them: each limited to the
// platforms its version (from pins) has a wheel for on python, by a marker,
// and left out when it has none. A root pins has no version for is listed as
// it is.
func withPlatforms(ctx context.Context, roots, pins []string, python string) ([]string, error) {
	versions := map[string]string{}
	for _, pin := range pins {
		if name, v, ok := strings.Cut(pin, "=="); ok {
			versions[manifest.DistName(name)] = strings.TrimSpace(v)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, wheelsTimeout)
	defer cancel()
	where := make([]int, len(roots))
	errs := make([]error, len(roots))
	var wg sync.WaitGroup
	for i, root := range roots {
		v, ok := versions[root]
		if !ok {
			where[i] = onLinux | onDarwin
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			where[i], errs[i] = wheelsOf(ctx, root, v, python)
		}()
	}
	wg.Wait()
	var out []string
	for i, root := range roots {
		switch {
		case errs[i] != nil:
			return nil, fmt.Errorf("reading which platforms %s has wheels for: %w", root, errs[i])
		case where[i] == onLinux|onDarwin:
			out = append(out, root)
		case where[i] == onLinux:
			out = append(out, root+"; sys_platform == 'linux'")
		case where[i] == onDarwin:
			out = append(out, root+"; sys_platform == 'darwin'")
		}
	}
	return out, nil
}

// wheelsOf is where dist's version has a wheel for on python: from the cache,
// else from PyPI's index page for dist, then kept.
func wheelsOf(ctx context.Context, dist, version, python string) (int, error) {
	dir := cacheDir(wheelsCacheDir)
	name := dist + "==" + version + "-python-" + python
	if dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				return n, nil
			}
		}
	}
	files, err := indexFiles(ctx, dist)
	if err != nil {
		return 0, err
	}
	where := wheelPlatforms(files, version, python)
	if dir != "" && os.MkdirAll(dir, freezeDirPerm) == nil {
		_ = fsatomic.WriteFile(filepath.Join(dir, name), []byte(strconv.Itoa(where)), freezeFilePerm) //nolint:errcheck // a cache that cannot be written is not a failure
	}
	return where, nil
}

// indexFiles reads the file names PyPI's simple index lists for dist.
func indexFiles(ctx context.Context, dist string) ([]string, error) {
	url := pypiSimpleURL + "/" + dist + "/"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.pypi.simple.v1+json")
	req.Header.Set("User-Agent", "astro-cli/"+version.Current())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reading %s: %s", url, resp.Status)
	}
	var page struct {
		Files []struct {
			Filename string `json:"filename"`
		} `json:"files"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, wheelsMaxBytes)).Decode(&page); err != nil {
		return nil, fmt.Errorf("reading %s: %w", url, err)
	}
	files := make([]string, len(page.Files))
	for i, f := range page.Files {
		files[i] = f.Filename
	}
	return files, nil
}

// wheelPlatforms is where the wheels among files of version install on python
// ("3.14"): onLinux for a manylinux or musllinux wheel of x86_64 or aarch64,
// onDarwin for a macOS wheel of arm64 or universal2, both for a pure-Python
// one. A wheel's Python tag has to be py3 or this CPython, with no ABI, the
// stable ABI of an older CPython, or this CPython's own (not its free-threaded
// build).
func wheelPlatforms(files []string, version, python string) int {
	minor, err := strconv.Atoi(strings.TrimPrefix(python, "3."))
	if err != nil || !strings.HasPrefix(python, "3.") {
		return 0
	}
	cp := "cp3" + strconv.Itoa(minor)
	where := 0
	for _, f := range files {
		parts := strings.Split(strings.TrimSuffix(f, ".whl"), "-")
		if !strings.HasSuffix(f, ".whl") || len(parts) < 5 || parts[1] != version {
			continue
		}
		pyTags, abi, platforms := parts[len(parts)-3], parts[len(parts)-2], parts[len(parts)-1]
		if !pythonFits(pyTags, abi, cp, minor) {
			continue
		}
		for _, p := range strings.Split(platforms, ".") {
			switch {
			case p == "any":
				where |= onLinux | onDarwin
			case (strings.HasPrefix(p, "manylinux") || strings.HasPrefix(p, "musllinux")) &&
				(strings.HasSuffix(p, "_x86_64") || strings.HasSuffix(p, "_aarch64")):
				where |= onLinux
			case strings.HasPrefix(p, "macosx_") && (strings.HasSuffix(p, "_arm64") || strings.HasSuffix(p, "_universal2")):
				where |= onDarwin
			}
		}
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
