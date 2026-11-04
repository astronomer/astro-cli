package local

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/plan"
	"github.com/astronomer/astro-cli/internal/project"
	"github.com/astronomer/astro-cli/pkg/localrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// stoppedAfterWatch is a runtime with nothing running, so the stop --watch
// ends with reports "already stopped" rather than failing.
type stoppedAfterWatch struct{ fakeRuntime }

func (stoppedAfterWatch) Attach(string) (localrt.Airflow, error) { return nil, localrt.ErrNotRunning }

func TestWatchText(t *testing.T) {
	files := []string{manifest.Marker}
	cases := []struct {
		name string
		ev   localrt.WatchEvent
		want string
	}{
		{
			"unreadable",
			localrt.WatchEvent{Kind: localrt.WatchUnreadable, Files: files, Err: errors.New("expected ]")},
			"pyproject.toml does not load, so Airflow keeps running as it was: expected ]",
		},
		{"a save that changed nothing says nothing", localrt.WatchEvent{Kind: localrt.WatchUnchanged, Files: files}, ""},
		{
			"a removal is said",
			localrt.WatchEvent{Kind: localrt.WatchUnchanged, Files: files, Change: localrt.Change{Removed: []string{"requests"}}},
			"no longer declared, and still installed until the next restart: requests",
		},
		{
			"installing",
			localrt.WatchEvent{Kind: localrt.WatchInstalling, Files: files, Change: localrt.Change{Install: []string{"requests", "httpx"}}},
			"pyproject.toml changed: installing requests, httpx",
		},
		{
			"installed",
			localrt.WatchEvent{Kind: localrt.WatchInstalled, Change: localrt.Change{Install: []string{"requests"}}},
			"installed requests",
		},
		{
			"install failed",
			localrt.WatchEvent{Kind: localrt.WatchInstallFailed, Err: errors.New("no solution found")},
			"the install failed, so Airflow keeps running as it was: no solution found",
		},
		{"restarting names every reason", localrt.WatchEvent{Kind: localrt.WatchRestarting, Files: []string{".env", manifest.Marker}, Change: localrt.Change{
			Reasons: []localrt.Reason{{Code: localrt.ReasonDotenv}, {Code: localrt.ReasonRepinned, Dist: "pandas"}},
		}}, ".env and pyproject.toml changed: restarting, because .env changed; the requirement on pandas changed, and it is already installed in the running Airflow"},
		{"the restart prints its own status", localrt.WatchEvent{Kind: localrt.WatchRestarted}, ""},
		{
			"restart failed",
			localrt.WatchEvent{Kind: localrt.WatchRestartFailed, Err: errors.New("port in use")},
			"the restart failed: port in use. Save again once it is fixed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, watchText(tc.ev))
		})
	}
}

// A script reads codes, not sentences: the json event carries the localrt
// kind and the reason codes as they are.
func TestReportWatchJSON(t *testing.T) {
	out := &bytes.Buffer{}
	reportWatch(cliout.Renderer{Format: cliout.FormatJSON, Out: out}, localrt.WatchEvent{
		Kind:  localrt.WatchRestarting,
		Files: []string{manifest.Marker},
		Change: localrt.Change{
			Action:  localrt.ChangeRestart,
			Removed: []string{"requests"},
			Reasons: []localrt.Reason{{Code: localrt.ReasonRepinned, Dist: "pandas"}},
		},
	})
	var got map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &got))
	require.Equal(t, map[string]any{
		"event":   "watch",
		"watch":   "restarting",
		"files":   []any{manifest.Marker},
		"removed": []any{"requests"},
		"reasons": []any{map[string]any{"code": "repinned", "dist": "pandas"}},
	}, got)
}

// --watch ends by stopping what it started, whether the watch ended because
// of an interrupt or never began.
func TestRunWatchStopsAirflowWhenTheWatchEnds(t *testing.T) {
	for _, watchErr := range []error{nil, errors.New("too many open files")} {
		d, stdout := testDeps(t)
		d.Runtime = stoppedAfterWatch{}
		// The stop finds the project from the working directory.
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, manifest.Marker), []byte("[tool.astro]\n"), 0o600))
		d.WorkingDir = func() (string, error) { return dir, nil }
		var got localrt.WatchOptions
		d.Watch = func(_ context.Context, o localrt.WatchOptions) error {
			got = o
			o.OnEvent(localrt.WatchEvent{Kind: localrt.WatchInstalled, Change: localrt.Change{Install: []string{"requests"}}})
			return watchErr
		}
		m := &manifest.Manifest{}
		c := &cli{d: d, output: cliout.FormatText}
		err := c.runWatch(context.Background(), &plan.Built{
			Project:  &project.Project{Dir: "/p"},
			Plan:     localrt.Plan{Mode: localrt.ModeStandalone},
			Manifest: m,
		}, false, nil)
		if watchErr != nil {
			require.ErrorIs(t, err, watchErr)
		} else {
			require.NoError(t, err)
		}
		require.Equal(t, "/p", got.ProjectPath)
		require.Equal(t, localrt.ModeStandalone, got.Mode)
		require.Same(t, m, got.Manifest, "the first edit is compared with what the start was built from")
		require.NotNil(t, got.Restart)
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		require.Equal(t, []string{"installed requests", "airflow: already stopped"}, lines)
		require.Contains(t, stderrOf(d), "Press Ctrl-C to stop Airflow and exit")
	}
}

// A restart the watch asks for after Airflow went down is a fresh start, and
// still the watch's: tied to the session, on the watch's port, and in its
// mode when no record survives to name one.
func TestWatchRestartOfAStoppedProjectKeepsTheWatchsStart(t *testing.T) {
	for _, mode := range []localrt.Mode{localrt.ModeStandalone, localrt.ModeDocker} {
		d, _, calls := stoppedProject(t, "")
		c := &cli{d: d, output: cliout.FormatText}
		_, err := c.restartProject(context.Background(), restartOptions{stopWithSession: true, port: 8123, mode: mode})
		require.ErrorIs(t, err, localrt.ErrNotImplemented, "the fake runtime refuses the start it was asked for")
		require.True(t, calls.started.StopWithSession, "%s: the start is tied to the watch", mode)
		require.Equal(t, 8123, calls.started.RequestedPort, mode)
		require.Equal(t, mode, calls.started.Mode)
	}
}

// --watch with nothing to watch with is refused before Airflow starts, not
// after a start it would leave behind.
func TestStartWatchWithoutAWatcherRefusesBeforeStarting(t *testing.T) {
	d, _, calls := stoppedProject(t, "")
	err := execute(t, d, "local", "start", "--watch")
	require.ErrorIs(t, err, errWatchUnavailable)
	require.Empty(t, calls.started.ProjectPath, "nothing was started")
}

func TestRunWatchRefusesWithoutAWatcher(t *testing.T) {
	d, _ := testDeps(t)
	c := &cli{d: d, output: cliout.FormatText}
	err := c.runWatch(context.Background(), &plan.Built{Project: &project.Project{Dir: "/p"}}, false, nil)
	require.ErrorContains(t, err, "--watch is not available")
}
