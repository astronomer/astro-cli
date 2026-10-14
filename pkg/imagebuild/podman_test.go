package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEngine writes an executable named name that prints out for --version.
func fakeEngine(t *testing.T, name, out string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake engine is a shell script")
	}
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\necho '"+out+"'\n"), 0o700))
	return p
}

// The engine says what it is, whatever its binary is called: the
// podman-docker shim is named docker and is podman.
func TestIsPodman(t *testing.T) {
	ctx, cmd := context.Background(), NewExecCommander()
	assert.True(t, IsPodman(ctx, cmd, fakeEngine(t, "docker", "podman version 5.2.0"), nil), "the podman-docker shim")
	assert.True(t, IsPodman(ctx, cmd, fakeEngine(t, "podman", "podman version 5.2.0"), nil))
	assert.False(t, IsPodman(ctx, cmd, fakeEngine(t, "docker", "Docker version 27.3.1, build ce12230"), nil))
	assert.False(t, IsPodman(ctx, cmd, filepath.Join(t.TempDir(), "missing"), nil), "an engine that does not answer")
	assert.True(t, IsPodman(ctx, cmd, filepath.Join(t.TempDir(), "podman"), nil), "a podman that does not answer is still podman by name")
	assert.False(t, IsPodman(ctx, cmd, "", nil))
}
