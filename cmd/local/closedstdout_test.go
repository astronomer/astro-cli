package local

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// closedStdout is what os.Stdout returns once its reader has gone.
type closedStdout struct{}

func (closedStdout) Write([]byte) (int, error) {
	return 0, &fs.PathError{Op: "write", Path: os.Stdout.Name(), Err: syscall.EPIPE}
}

// streamingAirflow follows logs until its context ends, a line at a time.
type streamingAirflow struct{ lines int }

func (*streamingAirflow) Stop(context.Context, localrt.StopOptions) error    { return nil }
func (*streamingAirflow) Status() (localrt.Status, error)                    { return localrt.Status{}, nil }
func (*streamingAirflow) Run(context.Context, []string, localrt.Stdio) error { return nil }
func (*streamingAirflow) Shell(context.Context, localrt.Stdio) error         { return nil }
func (*streamingAirflow) Env() ([]string, error)                             { return nil, nil }

func (a *streamingAirflow) Logs(ctx context.Context, opts localrt.LogOptions) error {
	for i := 0; i < 1000; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		a.lines++
		opts.OnLine(localrt.LogLine{Component: "scheduler", Time: time.Now(), Text: "line"})
	}
	return errors.New("kept streaming to nobody")
}

type logRuntime struct {
	fakeRuntime
	af localrt.Airflow
}

func (r logRuntime) LogSource(string) (localrt.Airflow, error) { return r.af, nil }

func projectDeps(t *testing.T) Deps {
	t.Helper()
	d, _ := testDeps(t)
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "pyproject.toml"), []byte("[project]\n"), 0o600))
	d.WorkingDir = func() (string, error) { return project, nil }
	return d
}

// A follow stops on the first line it cannot write: the run ends with that
// write error, which is a closed stdout (exit 0), instead of streaming to
// nobody.
func TestLogsFollowStopsWhenStdoutIsClosed(t *testing.T) {
	af := &streamingAirflow{}
	d := projectDeps(t)
	d.Stdout = closedStdout{}
	d.Runtime = logRuntime{af: af}

	err := execute(t, d, "local", "logs", "-f", "-o", "json")
	require.Error(t, err)
	assert.True(t, cliout.ClosedStdout(err), "%v", err)
	assert.Equal(t, 0, cliout.ExitCode(context.Background(), err))
	assert.Equal(t, 1, af.lines, "the stream ended at the first write that failed")
}

// A watch line that cannot be written ends the watch the same way.
func TestAWatchEventThatCannotBeWrittenEndsTheWatch(t *testing.T) {
	var failed error
	reportWatch(cliout.Renderer{Format: cliout.FormatJSON, Out: closedStdout{}}, localrt.WatchEvent{Kind: localrt.WatchUnreadable, Files: []string{manifest.Marker}, Err: errors.New("x")},
		func(err error) { failed = err })
	require.Error(t, failed)
	assert.True(t, cliout.ClosedStdout(failed))
}
