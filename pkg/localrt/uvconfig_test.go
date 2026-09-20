//go:build !windows

package localrt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/uv"
)

// recordingUV writes an executable stand-in that answers --version and
// otherwise records the cache and the flags it was given.
func recordingUV(t *testing.T, record string) string {
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
	dir := t.TempDir()
	path := filepath.Join(dir, "uv")
	require.NoError(t, os.WriteFile(path, []byte(script), 0o700))
	return dir
}

// The three uv fields on Config have to arrive at the invocation, and the hop
// that carries them is three lines in New that the compiler cannot check:
// BinDir and CacheDir are both strings, so transposing them builds cleanly and
// every test that constructs UVOptions itself still passes. Shipped, that gives
// "uv not found" from a bundled copy while the cache is written into the
// binary's directory.
//
// So this goes in at the front door — a Config — and reads what the child saw.
// It is the same argument the engine's own UVOptions test makes one layer down,
// which is where HermeticEnv was found to be an option nothing could reach.
func TestTheUVFieldsOnConfigReachTheInvocation(t *testing.T) {
	record := filepath.Join(t.TempDir(), "env")
	binDir := recordingUV(t, record)
	cache := t.TempDir()
	project := t.TempDir()

	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	// Cleared so discovery has to reach BinDir; it is checked first.
	t.Setenv(uv.EnvBin, "")

	rtime := New(Config{
		RoutesDir:  t.TempDir(),
		UVBinDir:   binDir,
		UVCacheDir: cache,
		UVNoConfig: true,
	})

	require.NoError(t, rtime.Sync(t.Context(), Plan{ProjectPath: project, Mode: ModeStandalone}, Callbacks{}))

	got, err := os.ReadFile(record)
	require.NoError(t, err, "the binary in UVBinDir never ran, so that field does not reach uv")
	out := string(got)

	require.Contains(t, out, "cache="+cache,
		"UVCacheDir did not reach uv: %s", strings.TrimSpace(out))
	require.Contains(t, out, "--no-config",
		"UVNoConfig did not reach uv: %s", strings.TrimSpace(out))
	// Transposing BinDir and CacheDir in New would satisfy the first assertion
	// by accident, so pin that the cache is NOT the binary's directory.
	require.NotContains(t, out, "cache="+binDir,
		"the cache is the bin directory; the two fields are crossed in New")
}
