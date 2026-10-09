//go:build !windows

package proxy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/config"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
	"github.com/astronomer/astro-cli/version"
)

// setupTestDir points config.HomeConfigPath at a temp dir so Routes() and the
// daemon files live under it.
func setupTestDir(t *testing.T) {
	t.Helper()
	orig := config.HomeConfigPath
	config.HomeConfigPath = t.TempDir()
	t.Cleanup(func() {
		config.HomeConfigPath = orig
	})
}

// The lifecycle is pkg/proxy's and tested there. What is this CLI's is how it
// configures one: the store under the config home, its own binary re-executed
// into ServeSubcommand, its version, and the 1.x takeover before a start.
func TestDaemonIsThisCLIs(t *testing.T) {
	setupTestDir(t)
	origVersion := version.CurrVersion
	version.CurrVersion = "2.3.4"
	t.Cleanup(func() { version.CurrVersion = origVersion })

	d := daemon()

	assert.Equal(t, filepath.Join(config.HomeConfigPath, proxyDir), d.Store.Dir())
	assert.Equal(t, filepath.Join(config.HomeConfigPath, proxyDir, "proxy.pid"), d.RecordPath())
	exe, err := os.Executable()
	require.NoError(t, err)
	assert.Equal(t, exe, d.Exe)
	assert.Equal(t, []string{ServeSubcommand}, d.ServeArgs)
	assert.Equal(t, "2.3.4", d.Version)
	assert.NotNil(t, d.BeforeStart, "the 1.x takeover has to run before a start")
	assert.NotNil(t, d.Logf)
}

// BoundPort reads the record the daemon publishes, at the path this CLI's
// daemon uses.
func TestBoundPortReadsTheDaemonsRecord(t *testing.T) {
	setupTestDir(t)
	require.NoError(t, os.MkdirAll(Routes().Dir(), 0o755))
	require.NoError(t, pkgproxy.WriteRecord(daemon().RecordPath(), pkgproxy.Record{PID: os.Getpid(), Port: "16123"}))

	assert.Equal(t, "16123", BoundPort())
}
