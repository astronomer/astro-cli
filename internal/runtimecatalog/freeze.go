package runtimecatalog

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
	"github.com/astronomer/astro-cli/version"
)

// Two files the runtime team publishes beside every build, one per build and
// Python, both name==version lines:
//
//	https://cdn.astronomer.io/runtime-constraints/runtime-<build>-python-<python>.txt
//	https://cdn.astronomer.io/runtime-freeze/runtime-<build>-python-<python>.txt
//
// The constraints file is what the image's own install holds packages to
// (/etc/pip-constraints.txt in the image: Airflow, the Task SDK and, on
// Airflow 3, SQLAlchemy). init and a start hold a project's lockfile to it
// (RuntimeConstraints). The freeze is pip freeze of the whole image, which
// astro local check reads to say a missing module is one the image ships
// (RuntimeFreeze).
//
// Every Airflow 3 build has both, and Airflow 2 builds from 11.0.0; older ones
// answer 404, which reads as "unknown". A build's files never change, so a
// copy once read is kept under the cache root without an expiry, and reading
// it again costs no request. A 404 is kept too, for freezeMissingTTL.

// pinFile is one of the two published files: where it is served, the
// variable that points it at a mirror, and the cache directory its copies
// sit in, named as the CDN path is.
type pinFile struct {
	baseURL  string
	env      string
	cacheDir string
}

// ConstraintsURLEnv and FreezeURLEnv point the two files at a mirror or an
// air-gapped network, as ASTRO_RUNTIME_VERSIONS_URL does the catalog's address
// and ASTRO_AIRFLOW_INDEX_URL the index's. The files are looked for under them
// by the CDN's names, runtime-<build>-python-<python>.txt.
const (
	ConstraintsURLEnv = "ASTRO_RUNTIME_CONSTRAINTS_URL"
	FreezeURLEnv      = "ASTRO_RUNTIME_FREEZE_URL"
)

// The two files. Variables so a test can point them at a local server.
var (
	constraintsFile = pinFile{"https://cdn.astronomer.io/runtime-constraints", ConstraintsURLEnv, "runtime-constraints"}
	freezeFile      = pinFile{"https://cdn.astronomer.io/runtime-freeze", FreezeURLEnv, "runtime-freeze"}
)

// base is the address f is read from: its variable when set, else the CDN.
func (f pinFile) base() string {
	if u := strings.TrimSpace(os.Getenv(f.env)); u != "" {
		return strings.TrimSuffix(u, "/")
	}
	return f.baseURL
}

const (
	// freezeTimeout bounds one request: a start or a check goes on without
	// the file, and an offline machine should not wait for it.
	freezeTimeout = 3 * time.Second
	// freezeMaxBytes caps the read. A freeze is a few KB.
	freezeMaxBytes = 1 << 20
	freezeFilePerm = 0o644
	freezeDirPerm  = 0o755
	// freezeMissingTTL is how long a 404 is remembered, so a project on a
	// runtime with no files, an Airflow 2 before 11.0.0, does not ask on
	// every start; long enough for that, short enough to notice a file
	// published later.
	freezeMissingTTL = 24 * time.Hour
	// freezeMissingSuffix names the empty file beside a cached copy that
	// records a 404 for it.
	freezeMissingSuffix = ".missing"
)

// freezeNamePart is what a build or a Python may contain to become part of a
// URL and a file name: "3.3-9", "12.6.0", "3.12". Anything else is refused
// before it reaches either.
var freezeNamePart = regexp.MustCompile(`^\d[0-9A-Za-z.+-]*$`)

// ErrNotPublished is a 404: this build and Python have no such file.
var ErrNotPublished = errors.New("no such file is published for this build")

// RuntimeConstraints returns the constraints runtime build's image running
// python installs under, as name==version lines in the file's order.
func RuntimeConstraints(ctx context.Context, build, python string) ([]string, error) {
	return readPins(ctx, constraintsFile, build, python)
}

// RuntimeFreeze returns every package runtime build's image running python
// has installed, as name==version lines in the freeze's order.
func RuntimeFreeze(ctx context.Context, build, python string) ([]string, error) {
	return readPins(ctx, freezeFile, build, python)
}

// readPins reads f for build and python, from the cache when a copy is kept
// there and from the CDN otherwise. It says why it has nothing:
// ErrNotPublished for a build and Python with no file; any other error when
// the file could not be read.
func readPins(ctx context.Context, f pinFile, build, python string) ([]string, error) {
	if !freezeNamePart.MatchString(build) || !freezeNamePart.MatchString(python) {
		return nil, fmt.Errorf("%w: build %q, Python %q", ErrNotPublished, build, python)
	}
	base := f.base()
	dir, name := cacheDirFor(f.cacheDir, base, f.baseURL), fileName(build, python)
	data, err := cachedFile(dir, name)
	if err != nil {
		return nil, err
	}
	if data == nil {
		fetchCtx, cancel := context.WithTimeout(ctx, freezeTimeout)
		defer cancel()
		if data, err = fetchAndKeep(fetchCtx, base, dir, name); err != nil {
			return nil, err
		}
	}
	// Only a copy with a pin in it is kept or fetched (cachedFile, fetchFile).
	return pinsOf(data)
}

// fileName is the name a build's file for one Python is served and kept as.
func fileName(build, python string) string {
	return "runtime-" + build + "-python-" + python + ".txt"
}

// cacheDirFor is sub under the cache root for what is read from base, or ""
// for no cache when there is no root. What a mirror (a base other than
// defaultBase) answers is kept apart, in a directory named by a hash of its
// address, so its copies and its 404s never answer for the CDN, nor the CDN's
// for it.
func cacheDirFor(sub, base, defaultBase string) string {
	root, err := localrt.CacheRoot()
	if err != nil {
		return ""
	}
	dir := filepath.Join(root, sub)
	if base != defaultBase {
		sum := sha256.Sum256([]byte(base))
		dir = filepath.Join(dir, "mirror-"+hex.EncodeToString(sum[:6]))
	}
	return dir
}

// cachedFile is what the cache under dir knows of name: the bytes of a whole
// copy with a pin in it, or ErrNotPublished while a 404 for it is remembered.
// nil and no error when it knows nothing, as with dir "".
func cachedFile(dir, name string) ([]byte, error) {
	if dir == "" {
		return nil, nil
	}
	path := filepath.Join(dir, name)
	// A copy that does not read, or holds no pin, is read again from the CDN.
	if data, err := fsatomic.ReadFile(path); err == nil {
		if pins, err := pinsOf(data); err == nil && len(pins) > 0 {
			return data, nil
		}
	}
	if info, err := os.Stat(path + freezeMissingSuffix); err == nil && time.Since(info.ModTime()) < freezeMissingTTL {
		return nil, ErrNotPublished
	}
	return nil, nil
}

// fetchAndKeep fetches name from the directory at base and keeps what it
// learned under dir: the file, or that there is none. A cache that cannot be
// written is not a failure: the read worked.
func fetchAndKeep(ctx context.Context, base, dir, name string) ([]byte, error) {
	data, err := fetchFile(ctx, base+"/"+name)
	if dir == "" || (err != nil && !errors.Is(err, ErrNotPublished)) || os.MkdirAll(dir, freezeDirPerm) != nil {
		return data, err
	}
	if err == nil {
		_ = fsatomic.WriteFile(filepath.Join(dir, name), data, freezeFilePerm) //nolint:errcheck // best effort, see above
	} else {
		_ = fsatomic.WriteFile(filepath.Join(dir, name+freezeMissingSuffix), nil, freezeFilePerm) //nolint:errcheck // best effort, see above
	}
	return data, err
}

// fetchFile reads one file from the CDN and returns its bytes. Only a whole
// file with a pin in it comes back, so nothing short of one is ever cached.
// An error names the address without its user and password.
func fetchFile(ctx context.Context, url string) ([]byte, error) {
	shown := runtimeversions.WithoutUser(url)
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
		return nil, ErrNotPublished
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("reading %s: %s", shown, resp.Status)
	}
	// One byte past the cap, so a file the cap would cut short is refused
	// rather than read as a shorter one.
	data, err := io.ReadAll(io.LimitReader(resp.Body, freezeMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	if len(data) > freezeMaxBytes {
		return nil, fmt.Errorf("reading %s: larger than %d bytes", shown, freezeMaxBytes)
	}
	pins, err := pinsOf(data)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", shown, err)
	}
	if len(pins) == 0 {
		// A 200 with no pin in it is a captive portal or an error page, not
		// one of these files, and caching it would answer every later read
		// wrongly.
		return nil, fmt.Errorf("reading %s: no packages in the response", shown)
	}
	return data, nil
}

// pinsOf reads a published file into its name==version lines, as they are
// written: the one reading of either file, which decides both whether a copy
// is one (a copy with no such line is not) and what it holds. A "name @ url"
// line, an editable install, a comment or anything else names no version, and
// is left out. A line too long to read is an error, not the end of the file.
func pinsOf(data []byte) ([]string, error) {
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
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return pins, nil
}
