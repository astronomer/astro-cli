package localdocker

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The container.binary pin reaches the engine docker mode starts with, read
// for the project being started. Before this the engine was resolved with an
// empty config, so a user with OrbStack and podman both installed got OrbStack
// whatever they had set.
func TestPreferredHonorsContainerBinaryPerProject(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "proxy"), nil, nil)
	e.connFor = func(bin string) engineConn { return engineConn{bin: bin} }
	var askedFor []string
	e.SetContainerBinary(func(projectPath string) string {
		askedFor = append(askedFor, projectPath)
		if projectPath == "/pinned" {
			return "podman"
		}
		return "docker"
	})

	conn, err := e.preferred("/pinned")
	require.NoError(t, err)
	assert.Equal(t, "podman", conn.bin)

	conn, err = e.preferred("/other")
	require.NoError(t, err)
	assert.Equal(t, "docker", conn.bin)

	assert.Equal(t, []string{"/pinned", "/other"}, askedFor)
}

// A nil func keeps auto-detection rather than leaving the engine with nothing
// to call.
func TestSetContainerBinaryNilKeepsAutoDetect(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "proxy"), nil, nil)
	e.SetContainerBinary(nil)
	assert.Empty(t, e.binaryFor("/any"))
}

// The whole-machine sweep has no one project, so its engine order comes from
// the global setting, asked for with "".
func TestEngineOrderSweepAsksForTheGlobalSetting(t *testing.T) {
	e := New(filepath.Join(t.TempDir(), "proxy"), nil, nil)
	e.connFor = func(bin string) engineConn { return engineConn{bin: bin} }
	var askedFor []string
	e.SetContainerBinary(func(projectPath string) string {
		askedFor = append(askedFor, projectPath)
		return "podman"
	})

	pref, other := e.engineOrder(t.Context(), "")
	assert.Equal(t, "podman", pref.bin)
	assert.Equal(t, binDocker, other)
	assert.Equal(t, []string{""}, askedFor)
}
