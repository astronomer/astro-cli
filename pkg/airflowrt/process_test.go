package airflowrt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveInEnvPath(t *testing.T) {
	// Create a fake binary
	dir := t.TempDir()
	binDir := filepath.Join(dir, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	fakeBin := filepath.Join(binDir, "mybin")
	require.NoError(t, os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755))

	// The separator has to be the platform's. ResolveInEnvPath splits with
	// filepath.SplitList, which is ";" on Windows — a hardcoded ":" made the
	// whole string one entry there, so the lookup found nothing and the test
	// failed on a rule the product code gets right.
	other := filepath.Join(string(filepath.Separator), "usr", "bin")
	env := []string{"PATH=" + binDir + string(os.PathListSeparator) + other}

	// Should resolve to our fake binary
	resolved := ResolveInEnvPath("mybin", env)
	assert.Equal(t, fakeBin, resolved)

	// A path with a separator in it is returned as-is, never looked up.
	withSep := filepath.Join(other, "env")
	resolved = ResolveInEnvPath(withSep, env)
	assert.Equal(t, withSep, resolved)

	// Unknown binary should be returned as-is
	resolved = ResolveInEnvPath("nonexistent-binary-xyz", env)
	assert.Equal(t, "nonexistent-binary-xyz", resolved)
}

func TestCheckPortAvailable(t *testing.T) {
	// Port 1 is privileged and likely not in use — should be available
	err := CheckPortAvailable("1")
	assert.NoError(t, err)
}
