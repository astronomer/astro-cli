//go:build !windows

package proxydaemon

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

// The lifecycle is pkg/proxy's and tested there. What is this CLI's is how it
// configures one: the store under the dir it is given, its own binary
// re-executed into ServeSubcommand, and its version. The 1.x takeover is
// pkg/proxy's.
func TestDaemonIsThisCLIs(t *testing.T) {
	dir := t.TempDir()
	origVersion := version.CurrVersion
	version.CurrVersion = "2.3.4"
	t.Cleanup(func() { version.CurrVersion = origVersion })

	d := daemon(dir)

	assert.Equal(t, dir, d.Store.Dir())
	assert.Equal(t, filepath.Join(dir, "proxy.pid"), d.RecordPath())
	exe, err := os.Executable()
	require.NoError(t, err)
	assert.Equal(t, exe, d.Exe)
	assert.Equal(t, []string{ServeSubcommand}, d.ServeArgs)
	assert.Equal(t, "2.3.4", d.Version)
	assert.NotNil(t, d.Logf)
}

// BoundPort reads the record the daemon publishes, at the path this CLI's
// daemon uses.
func TestBoundPortReadsTheDaemonsRecord(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, pkgproxy.WriteRecord(daemon(dir).RecordPath(), pkgproxy.Record{PID: os.Getpid(), Port: "16123"}))

	assert.Equal(t, "16123", BoundPort(dir))
}

// Dir is <astro home>/.astro/proxy: ASTRO_HOME when set, the user's home
// otherwise.
func TestDirFollowsAstroHome(t *testing.T) {
	astroHome := t.TempDir()
	t.Setenv("ASTRO_HOME", astroHome)
	assert.Equal(t, filepath.Join(astroHome, ".astro", "proxy"), Dir())

	t.Setenv("ASTRO_HOME", "")
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, ".astro", "proxy"), Dir())
}
