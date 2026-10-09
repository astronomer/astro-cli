//go:build !windows

package deploy

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	houston_mocks "github.com/astronomer/astro-cli/internal/platform/apc/houston/mocks"
)

// A dags directory that cannot be looked at is not one that is missing: the
// deploy fails on it, rather than skipping the upload as if there were none.
func TestDagsOnlyDeployFailsOnADagsDirectoryItCannotRead(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores the directory modes this failure is injected with")
	}
	parent := filepath.Join(t.TempDir(), "project")
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "dags"), 0o755))
	require.NoError(t, os.Chmod(parent, 0o000))
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	client := new(houston_mocks.ClientInterface)
	got, err := DagsOnlyDeploy(client, "ws", "dep", parent, nil, false, "", Options{Yes: true})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNoDagsDirectory)
	assert.ErrorIs(t, err, syscall.EACCES)
	assert.ErrorContains(t, err, "reading the dags directory")
	assert.Equal(t, "dep", got)
	assert.Empty(t, client.Calls, "no Houston call")
}

// A path through a file is not a directory, as a file named dags is not.
func TestDagsOnlyDeployRefusesADagsPathThroughAFile(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(parent, nil, 0o600))
	_, err := DagsOnlyDeploy(new(houston_mocks.ClientInterface), "ws", "dep", parent, nil, false, "", Options{Yes: true})
	assert.ErrorIs(t, err, ErrNoDagsDirectory)
}
