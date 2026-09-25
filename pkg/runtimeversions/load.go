package runtimeversions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// DefaultURL is Astronomer's public runtime catalog, the same document the
	// v1 CLI and Astro Desktop read. It needs no credentials.
	DefaultURL = "https://updates.astronomer.io/astronomer-runtime"
	// URLEnv overrides DefaultURL for every consumer, for an air-gapped mirror
	// or a test harness.
	URLEnv = "ASTRO_RUNTIME_VERSIONS_URL"
	// CacheFile is the file DefaultURL's catalog is kept in, inside the cache
	// directory the caller names. Its content is the document exactly as
	// served, so every reader of the file, including CLI builds that predate
	// this package and Astro Desktop, decodes it the same way. A catalog read
	// from any other address is cached beside it under a name of its own; see
	// CachePath.
	CacheFile = "runtime-versions.json"
	// CacheTTL is how long a cached copy is used without asking again. New
	// runtimes ship far more slowly than this.
	CacheTTL = 24 * time.Hour
	// DefaultTimeout bounds a fetch when Options.Timeout is zero.
	DefaultTimeout = 15 * time.Second

	// maxBytes caps the read. The document is under 100 KB.
	maxBytes = 8 << 20
	// The cache is world-readable: the content is public release data, not
	// anything of the user's.
	cacheDirPerm  = 0o755
	cacheFilePerm = 0o644
)

// Options configure a Load.
type Options struct {
	// CacheDir is where the catalog is kept between runs. The caller names it;
	// empty means no cache, so every Load fetches.
	CacheDir string
	// Timeout bounds the fetch. Zero means DefaultTimeout.
	Timeout time.Duration
	// UserAgent names the client, like "astro-cli/1.40.0" or
	// "astro-desktop/0.14.0". It is the only thing the request says about who
	// is asking: no user, project, token or path goes with it.
	UserAgent string
}

// Source says where a catalog, or a default, came from.
type Source string

const (
	// SourceCatalog is a copy fetched just now.
	SourceCatalog Source = "catalog"
	// SourceCache is a cached copy younger than CacheTTL.
	SourceCache Source = "cache"
	// SourceStaleCache is a cached copy older than CacheTTL, used because the
	// fetch failed.
	SourceStaleCache Source = "stale-cache"
	// SourceFallback is the answer compiled into the binary, used because no
	// current copy of the catalog could be read: the fetch failed and there was
	// no cache, or only a stale one behind the binary. Load never returns it;
	// Default does.
	SourceFallback Source = "fallback"
	// SourceCatalogEmpty is the answer compiled into the binary, used because
	// the catalog was read (fetched, or a fresh cache) and names no qualifying
	// Airflow 3 series. Load never returns it; Default does.
	SourceCatalogEmpty Source = "catalog-empty"
	// SourceBuiltIn is the answer compiled into the binary when no lookup was
	// made at all, because the caller gave no resolver. Neither Load nor
	// Default returns it; pkg/scaffold reports it.
	SourceBuiltIn Source = "built-in"
)

// Load returns the catalog, trying in order: a fresh cache, with no request; a
// fetch; a stale cache of any age. A failed fetch falling back to an old copy is
// the point of the cache: an old copy still names every runtime that existed
// when it was written, and a service blip should not stop a start or an init.
//
// It fails only when none of the three produced a catalog.
func Load(ctx context.Context, o Options) (*Catalog, Source, error) {
	url := URL()
	path := cachePath(o.CacheDir, url)
	if path != "" {
		if data, err := readFresh(path); err == nil {
			if c, err := Parse(data); err == nil {
				return c, SourceCache, nil
			}
		}
	}

	data, fetchErr := fetch(ctx, o, url)
	if fetchErr == nil {
		c, err := Parse(data)
		if err == nil {
			if path != "" {
				writeCache(path, data)
			}
			return c, SourceCatalog, nil
		}
		fetchErr = err
	}

	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if c, err := Parse(data); err == nil {
				return c, SourceStaleCache, nil
			}
		}
	}
	return nil, "", fmt.Errorf("reading the Astro Runtime versions: %w", fetchErr)
}

// URL is the catalog address a Load reads: URLEnv when it is set, else
// DefaultURL.
func URL() string {
	if u := strings.TrimSpace(os.Getenv(URLEnv)); u != "" {
		return u
	}
	return DefaultURL
}

func fetch(ctx context.Context, o Options, url string) ([]byte, error) {
	timeout := o.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if o.UserAgent != "" {
		req.Header.Set("User-Agent", o.UserAgent)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", url, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxBytes))
}

// CachePath is the file a Load with this cache directory keeps the catalog in,
// for the address it reads now (see URL): CacheFile for DefaultURL, and
// runtime-versions-<first 12 hex of sha256(url)>.json for any other. Empty when
// cacheDir is.
//
// Keyed by address so an override never reads, or writes, the shared default
// cache: a fresh copy of the public catalog must not be served in place of the
// mirror someone pointed ASTRO_RUNTIME_VERSIONS_URL at, and whatever that
// address serves must not land in the file Astro Desktop and every other CLI
// read as the public catalog.
func CachePath(cacheDir string) string {
	return cachePath(cacheDir, URL())
}

func cachePath(cacheDir, url string) string {
	if cacheDir == "" {
		return ""
	}
	if url == DefaultURL {
		return filepath.Join(cacheDir, CacheFile)
	}
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(cacheDir, "runtime-versions-"+hex.EncodeToString(sum[:])[:12]+".json")
}

// readFresh returns the cached bytes when they are younger than CacheTTL.
func readFresh(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > CacheTTL {
		return nil, errors.New("cached runtime versions are stale")
	}
	return os.ReadFile(path)
}

// writeCache stores the catalog for next time, through a temporary file and a
// rename, because Astro Desktop and the CLI share the file and one must never
// read the other's half-written copy. A cache that cannot be written is not a
// failure: the lookup already succeeded.
func writeCache(path string, data []byte) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, cacheDirPerm); err != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, CacheFile+".*.tmp")
	if err != nil {
		return
	}
	name := tmp.Name()
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Chmod(name, cacheFilePerm) != nil || os.Rename(name, path) != nil {
		_ = os.Remove(name) //nolint:errcheck // best effort: a stray temp file only costs disk
	}
}
