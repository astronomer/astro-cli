package runtimecatalog

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
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
// root without an expiry, and reading it again costs no request.

// freezeBaseURL is where the freeze files are served. A variable so a test can
// point it at a local server.
var freezeBaseURL = "https://cdn.astronomer.io/runtime-freeze"

const (
	// freezeCacheDir is the directory under the cache root the copies sit in,
	// named as the CDN path is.
	freezeCacheDir = "runtime-freeze"
	// freezeTimeout bounds every request of one RuntimeFreeze together: it runs
	// only to word a hint, and an offline machine should not wait for it.
	freezeTimeout = 3 * time.Second
	// freezeMaxBytes caps the read. A freeze is a few KB.
	freezeMaxBytes = 1 << 20
	freezeFilePerm = 0o644
	freezeDirPerm  = 0o755
)

// freezeNamePart is what a build or a Python may contain to become part of a
// URL and a file name: "3.3-9", "12.6.0", "3.12". Anything else is refused
// before it reaches either.
var freezeNamePart = regexp.MustCompile(`^\d[0-9A-Za-z.+-]*$`)

// errFreezeNotPublished is a 404: this build and Python have no freeze file.
var errFreezeNotPublished = errors.New("no freeze file is published for this build")

// RuntimeFreeze returns the distributions the image of runtime build has
// installed, normalized (manifest.DistName), for the first of pythons that has
// a freeze file: the caller lists the Python it expects first and fallbacks
// after, since the catalog lists no Pythons for an Airflow 2 build. nil when
// none could be read: no build, a build too old to have one, or no network
// and no cached copy.
//
// Each Python is tried in the caller's order, its cached copy and then the
// CDN, so a copy kept for a fallback never stands in for the Python expected
// while the expected one can still be read. A failure other than a 404 ends
// the requests, so an offline run spends one timeout, not one per Python;
// what is left is answered from the cache alone, so offline still finds a
// copy kept for a later Python.
func RuntimeFreeze(ctx context.Context, build string, pythons []string) []string {
	if !freezeNamePart.MatchString(build) {
		return nil
	}
	dir := ""
	if root, err := localrt.CacheRoot(); err == nil {
		dir = filepath.Join(root, freezeCacheDir)
	}
	ctx, cancel := context.WithTimeout(ctx, freezeTimeout)
	defer cancel()
	online := true
	for _, py := range pythons {
		if !freezeNamePart.MatchString(py) {
			continue
		}
		name := "runtime-" + build + "-python-" + py + ".txt"
		if dists := cachedFreeze(dir, name); dists != nil {
			return dists
		}
		if !online {
			continue
		}
		data, dists, err := fetchFreeze(ctx, freezeBaseURL+"/"+name)
		switch {
		case errors.Is(err, errFreezeNotPublished):
			continue
		case err != nil:
			online = false
			continue
		}
		// A cache that cannot be written is not a failure: the read worked.
		if dir != "" && os.MkdirAll(dir, freezeDirPerm) == nil {
			_ = fsatomic.WriteFile(filepath.Join(dir, name), data, freezeFilePerm) //nolint:errcheck // best effort, see above
		}
		return dists
	}
	return nil
}

// cachedFreeze is the distributions of the copy of name kept under dir, or nil
// when there is none, or it does not parse, which the CDN then answers.
func cachedFreeze(dir, name string) []string {
	if dir == "" {
		return nil
	}
	data, err := fsatomic.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return nil
	}
	if dists, err := parseFreeze(data); err == nil && len(dists) > 0 {
		return dists
	}
	return nil
}

// fetchFreeze reads one freeze file from the CDN and returns its bytes, for
// the cache, and its distributions. Only a whole, parseable file with a
// package in it comes back, so nothing short of one is ever cached.
func fetchFreeze(ctx context.Context, url string) (data []byte, dists []string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "astro-cli/"+version.Current())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, nil, errFreezeNotPublished
	case resp.StatusCode != http.StatusOK:
		return nil, nil, fmt.Errorf("reading %s: %s", url, resp.Status)
	}
	// One byte past the cap, so a file the cap would cut short is refused
	// rather than read as a shorter freeze.
	data, err = io.ReadAll(io.LimitReader(resp.Body, freezeMaxBytes+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > freezeMaxBytes {
		return nil, nil, fmt.Errorf("reading %s: larger than %d bytes", url, freezeMaxBytes)
	}
	if dists, err = parseFreeze(data); err != nil {
		return nil, nil, fmt.Errorf("reading %s: %w", url, err)
	}
	if len(dists) == 0 {
		// A 200 with no requirement in it is a captive portal or an error page,
		// not a freeze, and caching it would answer every later read wrongly.
		return nil, nil, fmt.Errorf("reading %s: no packages in the response", url)
	}
	return data, dists, nil
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
