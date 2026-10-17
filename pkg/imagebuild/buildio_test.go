package imagebuild

import (
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// dirCmd is a fakeCmd that is also a DirCommander, recording the directory
// each RunIn was given.
type dirCmd struct {
	fakeCmd
	dirs []string
}

func (d *dirCmd) RunIn(ctx context.Context, dir string, env []string, s rt.Stdio, name string, args ...string) error {
	d.dirs = append(d.dirs, dir)
	return d.Run(ctx, env, s, name, args...)
}

// writeBoth is a build that writes a line, a blank line and a line to each
// stream.
func writeBoth(_ string, s rt.Stdio) error {
	fmt.Fprint(s.Out, "out 1\n\nout 2\n")
	fmt.Fprint(s.Err, "err 1\n\nerr 2\n")
	return nil
}

// Stdout and Stderr, when set, take the build's output raw, blank lines and
// all, and the callbacks see none of it.
func TestBuildStreamsRawToTheRequestsWriters(t *testing.T) {
	var lines []string
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }}
	var out, errOut bytes.Buffer
	req := dockerfileRequest(t)
	req.Stdout, req.Stderr = &out, &errOut

	_, err := testBuilder(&fakeCmd{run: writeBoth}).Build(context.Background(), req, cb)
	require.NoError(t, err)
	assert.Equal(t, "out 1\n\nout 2\n", out.String())
	assert.Equal(t, "err 1\n\nerr 2\n", errOut.String())
	assert.Empty(t, lines)
}

// Unset, the output reaches the callbacks as lines, as it always has; one
// writer set alone leaves the other stream there.
func TestBuildStreamsToCallbacksWithoutWriters(t *testing.T) {
	var lines []string
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l.Text) }}

	_, err := testBuilder(&fakeCmd{run: writeBoth}).Build(context.Background(), dockerfileRequest(t), cb)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"out 1", "out 2", "err 1", "err 2"}, lines)

	lines = nil
	var out bytes.Buffer
	req := dockerfileRequest(t)
	req.Stdout = &out
	_, err = testBuilder(&fakeCmd{run: writeBoth}).Build(context.Background(), req, cb)
	require.NoError(t, err)
	assert.Equal(t, "out 1\n\nout 2\n", out.String())
	assert.Equal(t, []string{"err 1", "err 2"}, lines)
}

// Dir runs the build there through a DirCommander; without Dir it runs
// through Run, as before.
func TestBuildRunsInDir(t *testing.T) {
	cmd := &dirCmd{}
	req := dockerfileRequest(t)
	req.Dir = req.Context
	_, err := New(cmd, func() time.Time { return fixedTime }).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, []string{req.Context}, cmd.dirs)
	require.Len(t, cmd.calls, 1)

	cmd = &dirCmd{}
	_, err = New(cmd, func() time.Time { return fixedTime }).Build(context.Background(), dockerfileRequest(t), rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, cmd.dirs)
	assert.Len(t, cmd.calls, 1)
}

// A runner that cannot set a directory is refused rather than run in the
// wrong one.
func TestBuildRefusesDirWithoutADirCommander(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)
	req.Dir = req.Context
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set a working directory")
	assert.Empty(t, cmd.calls)
}

// The production runner runs the command in the directory asked for. The
// command is this test binary, asked to print its working directory.
func TestExecCommanderRunInSetsTheDirectory(t *testing.T) {
	if os.Getenv("IMAGEBUILD_PRINT_CWD") == "1" {
		wd, _ := os.Getwd()
		fmt.Print(wd)
		os.Exit(0)
	}
	dir := t.TempDir()
	var out bytes.Buffer
	dc, ok := NewExecCommander().(DirCommander)
	require.True(t, ok, "the production runner is a DirCommander")
	err := dc.RunIn(context.Background(), dir, []string{"IMAGEBUILD_PRINT_CWD=1"}, rt.Stdio{Out: &out}, os.Args[0], "-test.run=^TestExecCommanderRunInSetsTheDirectory$")
	require.NoError(t, err)
	want, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	got, err := filepath.EvalSymlinks(strings.TrimSpace(out.String()))
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// A declared Dockerfile that is a directory is ErrDockerfileNotAFile, and one
// that is missing wraps fs.ErrNotExist, so a caller can name it its own way.
func TestBuildDockerfileErrorsCanBeToldApart(t *testing.T) {
	req := dockerfileRequest(t)
	req.Dockerfile = req.Context
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, ErrDockerfileNotAFile)
	assert.Equal(t, fmt.Sprintf("the Dockerfile this project declares (%s) is not a file", req.Context), err.Error())

	req.Dockerfile = filepath.Join(req.Context, "missing")
	_, err = testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.NotErrorIs(t, err, ErrDockerfileNotAFile)
}
