package imagebuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// releasesJSON is a trimmed copy of the version service's answer: two Airflow
// releases served by several runtimes each, plus a yanked one that must never
// be chosen.
const releasesJSON = `{
  "runtimeVersions": {
    "9.1.0":   {"metadata": {"airflowVersion": "2.7.1", "channel": "deprecated"}},
    "11.10.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "deprecated"}},
    "11.12.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "deprecated"}},
    "11.9.0":  {"metadata": {"airflowVersion": "2.9.2", "channel": "deprecated"}},
    "13.9.0":  {"metadata": {"airflowVersion": "2.11.2", "channel": "stable"}},
    "13.99.0": {"metadata": {"airflowVersion": "2.9.3", "channel": "stable", "yanked": true}}
  },
  "runtimeVersionsV3": {
    "3.1-2": {"metadata": {"airflowVersion": "3.1.0", "channel": "stable"}}
  }
}`

// withService points the resolver at canned bytes and hands back an empty cache
// directory to resolve against, so no test reaches the network or the user's
// home. The count is how many times the service was asked.
func withService(t *testing.T, body string, err error) (calls *int, cacheDir string) {
	t.Helper()
	n := 0
	prev := fetchReleases
	fetchReleases = func(context.Context) ([]byte, error) {
		n++
		if err != nil {
			return nil, err
		}
		return []byte(body), nil
	}
	t.Cleanup(func() { fetchReleases = prev })
	return &n, t.TempDir()
}

func TestLocalRuntimeImageAirflow3IsUnchanged(t *testing.T) {
	calls, cache := withService(t, releasesJSON, nil)

	ref, err := LocalRuntimeImage(t.Context(), "3.1", cache)
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1", ref)
	assert.Zero(t, *calls, "Airflow 3 resolves from the pin alone, with no lookup")
}

func TestLocalRuntimeImageAirflow2(t *testing.T) {
	_, cache := withService(t, releasesJSON, nil)
	ctx := t.Context()

	// The newest runtime carrying the pinned Airflow wins, and 11.12.0 beats
	// 11.10.0 by segment, not by string order.
	ref, err := LocalRuntimeImage(ctx, "2.9.3", cache)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	// A partial pin matches every release under it, so 2.9 takes 2.9.3 over
	// 2.9.2 — and 13.99.0 is yanked, so it never wins.
	ref, err = LocalRuntimeImage(ctx, "2.9", cache)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	// A bare major pin takes the newest of the whole line.
	ref, err = LocalRuntimeImage(ctx, "2", cache)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:13.9.0", ref)
}

// Docker mode's database service runs `airflow db migrate`, which arrived in
// Airflow 2.7. Both sides of that floor: 2.7 resolves, 2.6 is refused with a
// sentence instead of a container that exits 2, and a pin naming no minor is
// not refused at all — it means the newest Airflow 2.
func TestLocalRuntimeImageAirflow2Floor(t *testing.T) {
	calls, cache := withService(t, releasesJSON, nil)
	ctx := t.Context()

	ref, err := LocalRuntimeImage(ctx, "2.7.1", cache)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:9.1.0", ref)

	_, err = LocalRuntimeImage(ctx, "2.6.3", cache)
	require.Error(t, err)
	assert.ErrorContains(t, err, "Airflow 2.7 or later")
	assert.ErrorContains(t, err, "2.6.3", "the message must name the pin the project carries")

	// A minor below the floor is refused however few segments follow it.
	_, err = LocalRuntimeImage(ctx, "2.6", cache)
	assert.ErrorContains(t, err, "Airflow 2.7 or later")

	before := *calls
	_, err = LocalRuntimeImage(ctx, "2.0.2", cache)
	require.Error(t, err)
	assert.Equal(t, before, *calls, "a pin below the floor is refused without a lookup")

	// A bare major means the newest Airflow 2, which is above the floor.
	ref, err = LocalRuntimeImage(ctx, "2", cache)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:13.9.0", ref)
}

func TestLocalRuntimeImageUnknownAirflow(t *testing.T) {
	_, cache := withService(t, releasesJSON, nil)

	// Above the 2.7 floor, so this reaches the lookup and finds nothing.
	_, err := LocalRuntimeImage(t.Context(), "2.8.4", cache)
	require.ErrorIs(t, err, ErrNoRuntimeForAirflow)
	assert.ErrorContains(t, err, "2.8.4")
}

func TestLocalRuntimeImageRejectsOtherMajors(t *testing.T) {
	_, cache := withService(t, releasesJSON, nil)

	_, err := LocalRuntimeImage(t.Context(), "1.10.15", cache)
	assert.ErrorContains(t, err, "Airflow 2 or Airflow 3")

	_, err = LocalRuntimeImage(t.Context(), "", cache)
	assert.ErrorContains(t, err, "no Airflow version")
}

func TestLocalRuntimeImageCachesTheAnswer(t *testing.T) {
	calls, cache := withService(t, releasesJSON, nil)
	ctx := t.Context()

	_, err := LocalRuntimeImage(ctx, "2.9.3", cache)
	require.NoError(t, err)
	_, err = LocalRuntimeImage(ctx, "2.11.2", cache)
	require.NoError(t, err)
	assert.Equal(t, 1, *calls, "the second lookup should read the cache")
}

// A caller that names no cache directory gets no cache: every lookup asks the
// service, and nothing is written anywhere.
func TestLocalRuntimeImageWithoutACache(t *testing.T) {
	calls, _ := withService(t, releasesJSON, nil)
	ctx := t.Context()

	ref, err := LocalRuntimeImage(ctx, "2.9.3", "")
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)

	_, err = LocalRuntimeImage(ctx, "2.9.3", "")
	require.NoError(t, err)
	assert.Equal(t, 2, *calls, "with no cache directory every lookup asks the service")
}

func TestLocalRuntimeImageFallsBackToAStaleCache(t *testing.T) {
	// A good answer lands in the cache first.
	_, dir := withService(t, releasesJSON, nil)
	_, err := LocalRuntimeImage(t.Context(), "2.9.3", dir)
	require.NoError(t, err)

	// Age it past the TTL, then take the network away. An old copy still names
	// every runtime that existed when it was written, so the start proceeds.
	path := filepath.Join(dir, versionsCacheFile)
	old := time.Now().Add(-2 * versionsCacheTTL)
	require.NoError(t, os.Chtimes(path, old, old))

	prevFetch := fetchReleases
	fetchReleases = func(context.Context) ([]byte, error) { return nil, errors.New("no network") }
	t.Cleanup(func() { fetchReleases = prevFetch })

	ref, err := LocalRuntimeImage(t.Context(), "2.9.3", dir)
	require.NoError(t, err)
	assert.Equal(t, "quay.io/astronomer/astro-runtime:11.12.0", ref)
}

func TestLocalRuntimeImageReportsAFailedLookup(t *testing.T) {
	_, cache := withService(t, "", errors.New("dial tcp: no route to host"))

	_, err := LocalRuntimeImage(t.Context(), "2.9.3", cache)
	require.Error(t, err)
	assert.ErrorContains(t, err, "Astro Runtime versions")
	assert.ErrorContains(t, err, "no route to host")
}

func TestLocalRuntimeImageRejectsAnEmptyServiceAnswer(t *testing.T) {
	_, cache := withService(t, `{"runtimeVersions": {}}`, nil)

	_, err := LocalRuntimeImage(t.Context(), "2.9.3", cache)
	assert.ErrorContains(t, err, "listed no runtimes")
}
