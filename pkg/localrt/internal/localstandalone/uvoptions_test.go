//go:build !windows

package localstandalone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
	"github.com/astronomer/astro-cli/pkg/uv"
)

// fakeUvBin writes an executable stand-in for uv that answers --version and
// otherwise records the environment it was given.
func fakeUvBin(t *testing.T, record string) string {
	t.Helper()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "uv 9.9.9 (0000000 2026-01-01 test)"
  exit 0
fi
echo "newer=${UV_EXCLUDE_NEWER:-unset}" > "` + record + `"
mkdir -p .venv
exit 0
`
	path := filepath.Join(t.TempDir(), "uv")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

// The consumer's uv preferences have to survive the trip from localrt.Config
// through the engine into the client that actually runs uv. Asserting the
// struct fields were copied would prove nothing the compiler does not already
// check, so this drives a real invocation and reads what the child saw.
func TestUVOptionsReachTheInvocation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		opts    UVOptions
		project string
		want    string
	}{
		{name: "hermetic strips it", opts: UVOptions{HermeticEnv: true}, want: "newer=unset"},
		{name: "default keeps it", opts: UVOptions{}, want: "newer=2020-01-01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "env")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv(uv.EnvBin, fakeUvBin(t, record))
			t.Setenv("UV_EXCLUDE_NEWER", "2020-01-01")

			client, err := uvClientFactory(tc.opts)(t.Context(), func(rt.LogLine) {})
			require.NoError(t, err)
			require.NoError(t, client.EnsureSynced(t.Context(), t.TempDir(), "", uv.Stdio{}))

			got, err := os.ReadFile(record)
			require.NoError(t, err)
			require.Equal(t, tc.want, strings.TrimSpace(string(got)))
		})
	}
}

// The notice has to survive the whole trip — uv's hook, the factory's closure,
// the engine's emitter — and land on the callbacks a start was given. Asserting
// the wiring by reading fields would prove nothing the compiler does not; this
// drives a sync that fails once, and reads what a consumer would have seen.
//
// Without it the plumbing can be deleted and every package still compiles and
// passes, which is how HermeticEnv came to be an option nothing could reach.
func TestTheSyncRetryNoticeReachesTheCallbacks(t *testing.T) {
	project := t.TempDir()
	countFile := filepath.Join(t.TempDir(), "count")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(uv.EnvBin, writeRetryingUvBin(t, countFile))

	var lines []rt.LogLine
	emit := func(l rt.LogLine) { lines = append(lines, l) }

	client, err := uvClientFactory(UVOptions{})(t.Context(), emit)
	require.NoError(t, err)
	require.NoError(t, client.EnsureSynced(t.Context(), project, "", uv.Stdio{}))

	var notice string
	for _, l := range lines {
		if strings.Contains(l.Text, "rebuilt from scratch") {
			notice = l.Text
			require.Equal(t, "uv", l.Component, "the notice belongs beside uv's own output")
		}
	}
	require.NotEmpty(t, notice, "a sync that was retried reported nothing: %+v", lines)
	require.Contains(t, notice, "Failed to read metadata", "the notice should carry what went wrong")
}

// writeRetryingUvBin fails the first sync the way a poisoned venv does, then
// succeeds.
func writeRetryingUvBin(t *testing.T, countFile string) string {
	t.Helper()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "uv 9.9.9 (0000000 2026-01-01 test)"
  exit 0
fi
c=0
[ -f "` + countFile + `" ] && c=$(cat "` + countFile + `")
echo $((c+1)) > "` + countFile + `"
mkdir -p .venv
if [ "$c" -eq 0 ]; then
  echo "error: Failed to read metadata from installed package" >&2
  exit 1
fi
exit 0
`
	path := filepath.Join(t.TempDir(), "uv")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

// recordingUvBin is fakeUvBin plus the two things the new options change about
// an invocation: the cache uv is told to use, and the flags it is given.
func recordingUvBin(t *testing.T, record string) string {
	t.Helper()
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "uv 9.9.9 (0000000 2026-01-01 test)"
  exit 0
fi
{
  echo "cache=${UV_CACHE_DIR:-unset}"
  echo "args=$*"
} > "` + record + `"
mkdir -p .venv
exit 0
`
	path := filepath.Join(t.TempDir(), "uv")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return path
}

// An embedder that ships its own uv has to be able to say where it is. The
// desktop bundles one inside its .app precisely so a user who never installed
// uv can still build an environment; without BinDir the search falls through
// to PATH and the installer locations and never finds it, and that user gets
// "uv not found" from the one feature the bundle exists for.
//
// Driven through a real invocation rather than by reading the field back: an
// option nothing reaches compiles and passes just as well, which is the
// failure the sibling test above was written for.
func TestABundledUVIsFoundThroughBinDir(t *testing.T) {
	record := filepath.Join(t.TempDir(), "env")
	bin := recordingUvBin(t, record)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Only the env var needs clearing: it is checked before BinDir, and BinDir
	// is checked before PATH. PATH is left alone deliberately — the stand-in is
	// a shell script and needs the ordinary tools to run, and stripping it to
	// prove a negative broke the script rather than the discovery. What proves
	// BinDir was used is the record file below, which only this binary writes.
	t.Setenv(uv.EnvBin, "")

	client, err := uvClientFactory(UVOptions{BinDir: filepath.Dir(bin)})(t.Context(), func(rt.LogLine) {})
	require.NoError(t, err, "uv was not found; a bundled copy is unreachable")
	require.NoError(t, client.EnsureSynced(t.Context(), t.TempDir(), "", uv.Stdio{}))

	_, err = os.Stat(record)
	require.NoError(t, err, "the binary in BinDir never ran")
}

// One cache, not two. An embedder that already has one wants the toolchain and
// the wheels downloaded once, not once per cache.
func TestTheEmbeddersCacheIsTheOneUVUses(t *testing.T) {
	record := filepath.Join(t.TempDir(), "env")
	own := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv(uv.EnvBin, recordingUvBin(t, record))

	client, err := uvClientFactory(UVOptions{CacheDir: own})(t.Context(), func(rt.LogLine) {})
	require.NoError(t, err)
	require.NoError(t, client.EnsureSynced(t.Context(), t.TempDir(), "", uv.Stdio{}))

	got, err := os.ReadFile(record)
	require.NoError(t, err)
	require.Contains(t, string(got), "cache="+own,
		"uv used a different cache from the embedder's, so downloads are paid twice")
}

// --no-config covers the files the way HermeticEnv covers the environment: a
// user's uv.toml, or a [tool.uv] table above the project, would otherwise steer
// a resolution whose python, dependencies and constraints the caller supplied.
func TestNoConfigReachesTheCommandLine(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts UVOptions
		want bool
	}{
		{"asked for", UVOptions{NoConfig: true}, true},
		{"left alone by default", UVOptions{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "env")
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv(uv.EnvBin, recordingUvBin(t, record))

			client, err := uvClientFactory(tc.opts)(t.Context(), func(rt.LogLine) {})
			require.NoError(t, err)
			require.NoError(t, client.EnsureSynced(t.Context(), t.TempDir(), "", uv.Stdio{}))

			got, err := os.ReadFile(record)
			require.NoError(t, err)
			require.Equal(t, tc.want, strings.Contains(string(got), "--no-config"),
				"args were %q", strings.TrimSpace(string(got)))
		})
	}
}

// The default branch, which the CLI and any embedder that names no cache both
// land on. Its path is a shared contract — cmd/local's check provisioner joins
// the same segment — so moving it silently orphans every cache already
// populated and re-downloads the toolchain on the next start.
func TestTheDefaultCacheIsSharedUnderTheAstroRoot(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", root)

	got, err := uvCacheDir(UVOptions{})

	require.NoError(t, err)
	require.Equal(t, "uv", filepath.Base(got),
		"the shared cache segment moved; every consumer that joined the old one now writes elsewhere")
	require.True(t, strings.HasPrefix(got, root), "got %q, want it under the astro cache root", got)
}

// uv runs with its working directory set to the project, so a relative cache
// would resolve per project — one cache per project, which is the outcome this
// option exists to prevent, and uv would create each one without complaint.
func TestARelativeCacheIsRefusedRatherThanResolved(t *testing.T) {
	_, err := uvCacheDir(UVOptions{CacheDir: "uv-cache"})

	require.Error(t, err)
	require.Contains(t, err.Error(), "absolute")
}
