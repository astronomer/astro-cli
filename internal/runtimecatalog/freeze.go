package runtimecatalog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/astronomer/astro-cli/pkg/fsatomic"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/version"
)

// What an Astro Runtime image has installed, read from the freeze file the
// runtime team publishes beside every build: `pip freeze` of the image, one
// file per build and Python, at
//
//	https://cdn.astronomer.io/runtime-freeze/runtime-<build>-python-<python>.txt
//
// Every Airflow 3 build has one, and Airflow 2 builds from 11.0.0; older ones
// answer 404, which reads as "unknown".
//
// A build's freeze never changes, so a copy once read is kept under the cache
// root without an expiry, and reading it again costs no request. A 404 is
// kept too, for freezeMissingTTL.

// freezeBaseURL is where the freeze files are served. A variable so a test can
// point it at a local server.
var freezeBaseURL = "https://cdn.astronomer.io/runtime-freeze"

// FreezeURLEnv overrides freezeBaseURL, for a mirror or an air-gapped
// network, as ASTRO_RUNTIME_VERSIONS_URL does the catalog's address and
// ASTRO_AIRFLOW_INDEX_URL the index's. The files are looked for under it by
// the CDN's names, runtime-<build>-python-<python>.txt.
const FreezeURLEnv = "ASTRO_RUNTIME_FREEZE_URL"

// freezeBase is the address freeze files are read from: FreezeURLEnv when it
// is set, else freezeBaseURL.
func freezeBase() string {
	if u := strings.TrimSpace(os.Getenv(FreezeURLEnv)); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return freezeBaseURL
}

// withoutUser is rawURL without its user and password, for an address an
// error prints: a mirror's address can carry its credentials.
func withoutUser(rawURL string) string {
	u, err := neturl.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	u.User = nil
	return u.String()
}

const (
	// freezeCacheDir is the directory under the cache root the copies sit in,
	// named as the CDN path is.
	freezeCacheDir = "runtime-freeze"
	// freezeTimeout bounds the request of one RuntimePins: a start goes on
	// without a seed, and an offline machine should not wait for it.
	freezeTimeout = 3 * time.Second
	// freezeMaxBytes caps the read. A freeze is a few KB.
	freezeMaxBytes = 1 << 20
	freezeFilePerm = 0o644
	freezeDirPerm  = 0o755
	// freezeMissingTTL is how long a 404 is remembered, so a project on a
	// runtime with no freeze, an Airflow 2 before 11.0.0, does not ask on
	// every start; long enough for that, short enough to notice a freeze
	// published later.
	freezeMissingTTL = 24 * time.Hour
	// freezeMissingSuffix names the empty file beside a freeze's cached copy
	// that records a 404 for it.
	freezeMissingSuffix = ".missing"
)

// freezeNamePart is what a build or a Python may contain to become part of a
// URL and a file name: "3.3-9", "12.6.0", "3.12". Anything else is refused
// before it reaches either.
var freezeNamePart = regexp.MustCompile(`^\d[0-9A-Za-z.+-]*$`)

// ErrFreezeNotPublished is a 404: this build and Python have no freeze file.
var ErrFreezeNotPublished = errors.New("no freeze file is published for this build")

// ErrPinnedResolution reports a RuntimePins made while UV_EXCLUDE_NEWER holds
// uv to a cutoff, a date or a duration alike. A freeze is of the present, and
// versions newer than the cutoff would leave uv nothing to lock, so none is read.
// A project's own [tool.uv] exclude-newer is the caller's to check, from the
// manifest.
var ErrPinnedResolution = errors.New("UV_EXCLUDE_NEWER holds uv to a cutoff, which a runtime's freeze does not follow")

// RuntimePins returns the freeze of runtime build's image running python as
// the name==version lines it lists, in its order, which init and a start
// seed a project's uv.lock with (scaffold.RuntimePins), from the cache when a
// copy is kept there and from the CDN otherwise.
//
// It says why it has nothing: ErrFreezeNotPublished for a build and Python
// with no file, which seeds nothing; ErrPinnedResolution under a
// UV_EXCLUDE_NEWER cutoff; any other error when the file could not be read.
func RuntimePins(ctx context.Context, build, python string) ([]string, error) {
	if strings.TrimSpace(os.Getenv("UV_EXCLUDE_NEWER")) != "" {
		return nil, ErrPinnedResolution
	}
	if !freezeNamePart.MatchString(build) || !freezeNamePart.MatchString(python) {
		return nil, fmt.Errorf("%w: build %q, Python %q", ErrFreezeNotPublished, build, python)
	}
	dir, name := freezeDir(), freezeName(build, python)
	data, err := cachedFreeze(dir, name)
	if err != nil {
		return nil, err
	}
	if data == nil {
		fetchCtx, cancel := context.WithTimeout(ctx, freezeTimeout)
		defer cancel()
		if data, err = fetchAndKeep(fetchCtx, freezeBase(), dir, name); err != nil {
			return nil, err
		}
	}
	return freezePins(data), nil
}

// freezeName is the file a build's freeze for one Python is served and kept
// as.
func freezeName(build, python string) string {
	return "runtime-" + build + "-python-" + python + ".txt"
}

// freezeDir is where freeze copies are kept: freezeCacheDir under the cache
// root, or "" for no cache when there is no root.
func freezeDir() string {
	return cacheDir(freezeCacheDir)
}

// cacheDir is sub under the cache root, or "" for no cache when there is no
// root.
func cacheDir(sub string) string {
	root, err := localrt.CacheRoot()
	if err != nil {
		return ""
	}
	return filepath.Join(root, sub)
}

// cachedFreeze is what the cache under dir knows of name: the bytes of a
// whole copy that parses, or ErrFreezeNotPublished while a 404 for it is
// remembered. nil and no error when it knows nothing, as with dir "".
func cachedFreeze(dir, name string) ([]byte, error) {
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, name)
	// A copy that does not parse is read again from the CDN.
	if data, err := fsatomic.ReadFile(path); err == nil {
		if dists, err := parseFreeze(data); err == nil && len(dists) > 0 {
			return data, nil
		}
	}
	if info, err := os.Stat(path + freezeMissingSuffix); err == nil && time.Since(info.ModTime()) < freezeMissingTTL {
		return nil, ErrFreezeNotPublished
	}
	return nil, nil
}

// fetchAndKeep fetches name from the directory at base (freezeBase, or
// rootsBase) and keeps what it learned under dir: the file, or that there is
// none. A cache that cannot be written is not a failure: the read worked.
func fetchAndKeep(ctx context.Context, base, dir, name string) ([]byte, error) {
	data, err := fetchFreeze(ctx, base+"/"+name)
	if dir == "" || (err != nil && !errors.Is(err, ErrFreezeNotPublished)) || os.MkdirAll(dir, freezeDirPerm) != nil {
		return data, err
	}
	if err == nil {
		_ = fsatomic.WriteFile(filepath.Join(dir, name), data, freezeFilePerm) //nolint:errcheck // best effort, see above
	} else {
		_ = fsatomic.WriteFile(filepath.Join(dir, name+freezeMissingSuffix), nil, freezeFilePerm) //nolint:errcheck // best effort, see above
	}
	return data, err
}

// fetchFreeze reads one freeze file from the CDN and returns its bytes. Only a
// whole, parseable file with a package in it comes back, so nothing short of
// one is ever cached. An error names the address without its user and
// password.
func fetchFreeze(ctx context.Context, url string) ([]byte, error) {
	shown := withoutUser(url)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("reading %s: the address does not parse", shown)
	}
	req.Header.Set("User-Agent", "astro-cli/"+version.Current())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		// The client's error names the address as requested; this names it
		// redacted, and keeps the cause.
		var urlErr *neturl.Error
		if errors.As(err, &urlErr) {
			err = urlErr.Err
		}
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrFreezeNotPublished
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("reading %s: %s", shown, resp.Status)
	}
	// One byte past the cap, so a file the cap would cut short is refused
	// rather than read as a shorter freeze.
	data, err := io.ReadAll(io.LimitReader(resp.Body, freezeMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	if len(data) > freezeMaxBytes {
		return nil, fmt.Errorf("reading %s: larger than %d bytes", shown, freezeMaxBytes)
	}
	dists, err := parseFreeze(data)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	if len(dists) == 0 {
		// A 200 with no requirement in it is a captive portal or an error page,
		// not a freeze, and caching it would answer every later read wrongly.
		return nil, fmt.Errorf("reading %s: no packages in the response", shown)
	}
	return data, nil
}

// parseFreeze reads `pip freeze` output into normalized distribution names:
// "name==version" and "name @ url" lines, comments and blanks skipped. A line
// too long to read is an error, not the end of the file.
func parseFreeze(data []byte) ([]string, error) {
	var dists []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), freezeMaxBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if name := manifest.DistName(line); name != "" {
			dists = append(dists, name)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return dists, nil
}

// freezePins reads `pip freeze` output, a copy parseFreeze accepted, into its
// name==version lines, as they are written. A "name @ url" line, an editable
// install or anything else names no version to hold a package to, and is left
// out.
func freezePins(data []byte) []string {
	var pins []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 64<<10), freezeMaxBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		name, release, ok := strings.Cut(line, "==")
		name, release = strings.TrimSpace(name), strings.TrimSpace(release)
		if !ok || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") ||
			name == "" || release == "" || strings.ContainsAny(name, " @;[") || strings.ContainsAny(release, " ;") {
			continue
		}
		pins = append(pins, name+"=="+release)
	}
	return pins
}
