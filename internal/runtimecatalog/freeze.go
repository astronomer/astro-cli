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
// A failure other than a 404 stops the search, so an offline run spends one
// timeout, not one per Python.
func RuntimeFreeze(ctx context.Context, build string, pythons []string) []string {
	if !freezeNamePart.MatchString(build) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, freezeTimeout)
	defer cancel()
	dir := ""
	if root, err := localrt.CacheRoot(); err == nil {
		dir = filepath.Join(root, freezeCacheDir)
	}
	for _, py := range pythons {
		if !freezeNamePart.MatchString(py) {
			continue
		}
		data, err := readFreeze(ctx, dir, "runtime-"+build+"-python-"+py+".txt")
		if errors.Is(err, errFreezeNotPublished) {
			continue
		}
		if err != nil {
			return nil
		}
		return parseFreeze(data)
	}
	return nil
}

// readFreeze returns the freeze file name, from the cache under dir when a
// copy is there, else from the CDN, keeping what it fetched. dir "" is no
// cache.
func readFreeze(ctx context.Context, dir, name string) ([]byte, error) {
	path := ""
	if dir != "" {
		path = filepath.Join(dir, name)
		if data, err := fsatomic.ReadFile(path); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	data, err := fetchFreeze(ctx, freezeBaseURL+"/"+name)
	if err != nil {
		return nil, err
	}
	if path != "" {
		// A cache that cannot be written is not a failure: the read worked.
		if os.MkdirAll(dir, freezeDirPerm) == nil {
			_ = fsatomic.WriteFile(path, data, freezeFilePerm) //nolint:errcheck // best effort, see above
		}
	}
	return data, nil
}

func fetchFreeze(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "astro-cli/"+version.Current())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, errFreezeNotPublished
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("reading %s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, freezeMaxBytes))
	if err != nil {
		return nil, err
	}
	if len(parseFreeze(data)) == 0 {
		// A 200 with no requirement in it is a captive portal or an error page,
		// not a freeze, and caching it would answer every later read wrongly.
		return nil, fmt.Errorf("reading %s: no packages in the response", url)
	}
	return data, nil
}

// parseFreeze reads `pip freeze` output into normalized distribution names:
// "name==version" and "name @ url" lines, comments and blanks skipped.
func parseFreeze(data []byte) []string {
	var dists []string
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "-") {
			continue
		}
		if name := manifest.DistName(line); name != "" {
			dists = append(dists, name)
		}
	}
	return dists
}
