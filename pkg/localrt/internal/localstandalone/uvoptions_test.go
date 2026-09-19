//go:build !windows

package localstandalone

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

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
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700)) //nolint:gosec // an executable stand-in under t.TempDir
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

			client, err := uvClientFactory(tc.opts)(t.Context())
			require.NoError(t, err)
			require.NoError(t, client.EnsureSynced(t.Context(), t.TempDir(), "", uv.Stdio{}))

			got, err := os.ReadFile(record) //nolint:gosec // a file this test just created under t.TempDir
			require.NoError(t, err)
			require.Equal(t, tc.want, strings.TrimSpace(string(got)))
		})
	}
}
