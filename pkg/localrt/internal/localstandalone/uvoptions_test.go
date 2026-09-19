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
