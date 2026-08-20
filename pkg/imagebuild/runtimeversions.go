package imagebuild

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Airflow 3 ships as Astro Runtime 3, whose tag is the Airflow version, so
// RuntimeImage builds that reference from the pin alone. Airflow 2 does not:
// its images are tagged by runtime version (11.12.0), and the Airflow release
// each one carries is only knowable from Astronomer's version service. So an
// Airflow 2 base image needs a lookup, and a lookup needs the network — which
// is why local Docker mode resolves through here and the pure RuntimeImage
// stays as it is.
//
// The Airflow 2 images answer to the same ONBUILD contract the build already
// relies on: COPY packages.txt, install-system-packages, COPY requirements.txt,
// install-python-dependencies. Nothing else about the build changes.

const (
	// runtimeReleaseURL is Astronomer's public version service, the same one
	// the v1 CLI reads. It needs no credentials.
	runtimeReleaseURL = "https://updates.astronomer.io/astronomer-runtime"
	// airflow2ImageRepo hosts the Astro Runtime images for Airflow 2.
	airflow2ImageRepo = "quay.io/astronomer/astro-runtime"
	// versionsCacheFile holds the last good copy of the service's answer,
	// inside the cache directory the caller names.
	versionsCacheFile = "runtime-versions.json"
	// versionsCacheTTL is how long a cached copy is used without asking the
	// service again. New runtimes ship far more slowly than this.
	versionsCacheTTL = 24 * time.Hour
	// versionsTimeout bounds the fetch, so a hung service delays a start by
	// seconds rather than holding it open.
	versionsTimeout = 15 * time.Second
	// maxReleasesBytes caps the read. The document is well under a megabyte.
	maxReleasesBytes = 8 << 20
	// cacheDirPerm and cacheFilePerm are world-readable: the content is public
	// release data, not anything of the user's.
	cacheDirPerm  = 0o755
	cacheFilePerm = 0o644
	// minAirflow2Minor is the oldest Airflow 2 local Docker mode runs. The
	// compose file's database service runs `airflow db migrate`, which arrived
	// in Airflow 2.7 and replaced `db upgrade`; below that the service exits 2
	// and every component then waits forever on a migration that never
	// completed. Everything under the floor is Astro Runtime 8 or older, out of
	// maintenance since 2023, so this refuses nothing that is still supported.
	minAirflow2Minor = 7
)

// ErrNoRuntimeForAirflow reports an Airflow version no published runtime
// carries. Callers branch with errors.Is.
var ErrNoRuntimeForAirflow = errors.New("no Astro Runtime carries this Airflow version")

// runtimeReleases is the slice of the version service's answer this package
// needs: runtime version → the Airflow it carries.
type runtimeReleases struct {
	RuntimeVersions map[string]struct {
		Metadata struct {
			AirflowVersion string `json:"airflowVersion"`
			Yanked         bool   `json:"yanked"`
		} `json:"metadata"`
	} `json:"runtimeVersions"`
}

// fetchReleases reads the version service. It is a package var so tests drive
// the resolver without a network.
var fetchReleases = func(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, versionsTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeReleaseURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s returned HTTP %d", runtimeReleaseURL, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxReleasesBytes))
}

// LocalRuntimeImage picks the base image local Docker mode builds FROM. Airflow
// 3 resolves exactly as the deploy path does. Airflow 2 is looked up in the
// version service, and the newest runtime carrying that Airflow wins, down to
// the 2.7 floor.
//
// cacheDir is where the service's answer is kept between runs, and the caller
// names it: this package has no business knowing where a consumer's cache
// lives. An empty cacheDir means no cache at all — every call fetches, which
// still works and only costs the request.
//
// This is the local start path only. The deploy path stays on RuntimeImage,
// which is Airflow 3 alone.
func LocalRuntimeImage(ctx context.Context, airflowVersion, cacheDir string) (string, error) {
	v := strings.TrimSpace(airflowVersion)
	if v == "" {
		return "", errors.New("no Airflow version was given; one is needed to pick a runtime image")
	}
	switch major, _, _ := strings.Cut(v, "."); major {
	case "3":
		return RuntimeImage(v)
	case "2":
		if err := checkAirflow2Floor(v); err != nil {
			return "", err
		}
		runtime, err := airflow2Runtime(ctx, v, cacheDir)
		if err != nil {
			return "", err
		}
		return airflow2ImageRepo + ":" + runtime, nil
	default:
		return "", fmt.Errorf("Docker mode runs Airflow 2 or Airflow 3, not %q", v)
	}
}

// checkAirflow2Floor refuses an Airflow 2 pin older than Docker mode runs,
// before the lookup, so the answer is a sentence rather than a database service
// that exits 2 in a container. A pin that names no minor ("2") is not refused:
// it means the newest Airflow 2, which is well above the floor.
func checkAirflow2Floor(version string) error {
	_, rest, found := strings.Cut(version, ".")
	if !found {
		return nil
	}
	minorText, _, _ := strings.Cut(rest, ".")
	minor, err := strconv.Atoi(minorText)
	if err != nil || minor >= minAirflow2Minor {
		return nil
	}
	return fmt.Errorf("Docker mode needs Airflow 2.%d or later, and this project pins %s: the metadata database is migrated with `airflow db migrate`, which does not exist before Airflow 2.%d",
		minAirflow2Minor, version, minAirflow2Minor)
}

// airflow2Runtime returns the newest published runtime version whose Airflow
// matches the pin. A partial pin ("2", "2.9") matches every release under it,
// the same way the pin itself is partial on purpose.
func airflow2Runtime(ctx context.Context, pin, cacheDir string) (string, error) {
	releases, err := loadReleases(ctx, cacheDir)
	if err != nil {
		return "", err
	}
	var best string
	for version, release := range releases.RuntimeVersions {
		if release.Metadata.Yanked {
			continue
		}
		if !matchesPin(release.Metadata.AirflowVersion, pin) {
			continue
		}
		if best == "" || compareVersions(version, best) > 0 {
			best = version
		}
	}
	if best == "" {
		return "", fmt.Errorf("%w: %s", ErrNoRuntimeForAirflow, pin)
	}
	return best, nil
}

// matchesPin reports whether an Airflow release satisfies the manifest's pin,
// which may name fewer segments than the release does.
func matchesPin(release, pin string) bool {
	return release == pin || strings.HasPrefix(release, pin+".")
}

// compareVersions orders two dotted numeric versions, segment by segment. The
// runtime versions this sees are plain "11.12.0" triples; a segment that is not
// a number sorts low rather than failing, so an unexpected tag never wins.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		an, bn := segment(as, i), segment(bs, i)
		if an != bn {
			if an < bn {
				return -1
			}
			return 1
		}
	}
	return 0
}

func segment(parts []string, i int) int {
	if i >= len(parts) {
		return 0
	}
	n, err := strconv.Atoi(parts[i])
	if err != nil {
		return -1
	}
	return n
}

// loadReleases returns the version service's answer, from the cache when it is
// fresh. A failed fetch falls back to a stale cache rather than stopping a
// start that would otherwise work: an old copy still names every runtime that
// existed when it was written. That fallback is the point of caching here —
// docker mode is the only route to OS packages and the only mode on Windows, so
// a service blip should not stop an Airflow 2 start.
//
// An empty path means the caller asked for no cache: fetch, and carry on.
func loadReleases(ctx context.Context, cacheDir string) (*runtimeReleases, error) {
	path := cachePath(cacheDir)
	if path != "" {
		if data, err := readFresh(path); err == nil {
			if releases, err := decodeReleases(data); err == nil {
				return releases, nil
			}
		}
	}

	data, fetchErr := fetchReleases(ctx)
	if fetchErr == nil {
		releases, err := decodeReleases(data)
		if err == nil {
			if path != "" {
				writeCache(path, data)
			}
			return releases, nil
		}
		fetchErr = err
	}

	// The fetch failed. Any cached copy beats no answer at all.
	if path != "" {
		if data, err := os.ReadFile(path); err == nil {
			if releases, err := decodeReleases(data); err == nil {
				return releases, nil
			}
		}
	}
	return nil, fmt.Errorf("reading the Astro Runtime versions: %w", fetchErr)
}

func decodeReleases(data []byte) (*runtimeReleases, error) {
	var releases runtimeReleases
	if err := json.Unmarshal(data, &releases); err != nil {
		return nil, err
	}
	if len(releases.RuntimeVersions) == 0 {
		return nil, errors.New("the version service listed no runtimes")
	}
	return &releases, nil
}

// cachePath is the file the answer is cached in, or "" when the caller named no
// cache directory.
func cachePath(cacheDir string) string {
	if cacheDir == "" {
		return ""
	}
	return filepath.Join(cacheDir, versionsCacheFile)
}

// readFresh returns the cached bytes when they are younger than the TTL.
func readFresh(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if time.Since(info.ModTime()) > versionsCacheTTL {
		return nil, errors.New("cached runtime versions are stale")
	}
	return os.ReadFile(path)
}

// writeCache stores the answer for next time. A cache that cannot be written is
// not a failure: the resolution already succeeded.
func writeCache(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), cacheDirPerm); err != nil {
		return
	}
	_ = os.WriteFile(path, data, cacheFilePerm) //nolint:errcheck // a failed cache write only costs the next lookup a fetch
}
