package localdocker

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// composeMarker writes the generated compose file for a project, which is what
// Clean reads as evidence that this project has ever run in docker mode.
func composeMarker(t *testing.T, projectPath string) string {
	t.Helper()
	dir, err := rt.StateDir(projectPath)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	path := filepath.Join(dir, composeFileName)
	require.NoError(t, os.WriteFile(path, []byte("services: {}\n"), 0o600))
	return path
}

// A project that never ran in docker mode is left entirely alone — no engine is
// even asked.
//
// Without the gate a standalone-only project reported "removed compose project
// astro-orders-demo-298a15 and its volumes": a removal of something that never
// existed, on the back of a container-engine probe it had no use for.
func TestCleanSkipsAProjectThatNeverRanInDockerMode(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)

	name, reached, err := e.Clean(context.Background(), t.TempDir())
	require.NoError(t, err)
	assert.Empty(t, name, "no compose project may be claimed for a project that never ran one")
	assert.True(t, reached, "nothing to ask about is not the same as docker being unreachable")
	assert.Empty(t, cmd.calls, "the gate must short-circuit before any engine is probed")
}

// The case the whole path exists for: containers already gone, volume still
// holding the metadata database, nothing running to discover the name from.
func TestCleanTearsDownVolumesUnderTheDerivedName(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composeFile := composeMarker(t, project)
	want, err := composeProjectName(project)
	require.NoError(t, err)

	name, reached, err := e.Clean(context.Background(), project)
	require.NoError(t, err)
	assert.Equal(t, want, name)
	assert.True(t, reached)

	// The same graceful window Stop gives containers, not a hard kill.
	assert.Contains(t, cmd.calls,
		"docker compose --project-name "+want+" down --timeout 10 --volumes --remove-orphans")
	assert.NoFileExists(t, composeFile, "a successful teardown retires the marker")
}

// A failed teardown must not destroy the evidence that this project is docker
// mode. Removing the compose file here would make every later reset skip the
// volume while still reporting a wipe — the database the user is trying to be
// rid of, silently kept forever.
func TestCleanKeepsTheComposeFileWhenTeardownFails(t *testing.T) {
	cmd := &fakeCmd{
		output: noProjects,
		run: func(call string, _ rt.Stdio) error {
			if strings.Contains(call, " down ") {
				return errors.New("no such volume")
			}
			return nil
		},
	}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composeFile := composeMarker(t, project)

	name, reached, err := e.Clean(context.Background(), project)
	require.Error(t, err, "a teardown that failed must not be reported as success")
	assert.Empty(t, name, "nothing was removed, so nothing may be named as removed")
	assert.True(t, reached, "the engine answered; it was the teardown that failed")
	assert.FileExists(t, composeFile, "the only evidence this project is docker mode must survive")
}

// No engine answered, so a docker-mode volume can neither be removed nor ruled
// out. Reporting that is the whole point of the second return value.
func TestCleanReportsDockerUnreachable(t *testing.T) {
	cmd := &fakeCmd{output: func(string) ([]byte, error) { return nil, errors.New("cannot connect to the daemon") }}
	e := testEngine(t, cmd)
	project := t.TempDir()
	composeFile := composeMarker(t, project)

	name, reached, err := e.Clean(context.Background(), project)
	require.NoError(t, err)
	assert.Empty(t, name)
	assert.False(t, reached, "no engine answered, so the caller must be told it could not tell")
	assert.FileExists(t, composeFile, "an unreachable engine leaves the marker for the next attempt")
}

// An engine that cannot be resolved is the same kind of "could not tell", and
// must not be reported as a completed wipe.
func TestCleanReportsAnEngineItCannotResolve(t *testing.T) {
	cmd := &fakeCmd{output: noProjects}
	e := testEngine(t, cmd)
	e.preferred = func(string) (engineConn, error) { return engineConn{}, errors.New("no container engine found") }
	project := t.TempDir()
	composeMarker(t, project)

	name, reached, err := e.Clean(context.Background(), project)
	require.ErrorContains(t, err, "no container engine found")
	assert.Empty(t, name)
	assert.False(t, reached, "having done nothing must never be reported as a complete wipe")
}
