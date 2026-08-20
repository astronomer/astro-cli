package imagebuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt"
)

// fakeCmd is a Commander that never touches a real daemon. Each call is
// recorded as "name arg arg..."; a hook drives the response and can read the
// wired stdio.
type fakeCmd struct {
	calls []string
	run   func(call string, s localrt.Stdio) error
}

func (f *fakeCmd) Run(_ context.Context, _ []string, s localrt.Stdio, name string, args ...string) error {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if f.run == nil {
		return nil
	}
	return f.run(call, s)
}

// fixedTime is the clock every builder under test carries, so log-line
// timestamps are deterministic.
var fixedTime = time.Date(2026, 7, 23, 0, 0, 0, 0, time.UTC)

// testBuilder builds a Builder with the fake command and the fixed clock.
func testBuilder(cmd *fakeCmd) *Builder {
	return New(cmd, func() time.Time { return fixedTime })
}

// testRequest is a build over the runtime base into a project work dir.
func testRequest(t *testing.T) Request {
	t.Helper()
	return Request{
		WorkDir:   t.TempDir(),
		BaseImage: "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:       "astro-local/my-project",
		Bin:       "docker",
	}
}

func hasCall(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestBuildWritesRequirements(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas==2.2.0", "requests"}

	image, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	// A build ran, tagged with the requested tag, and that tag is the image
	// returned to run.
	assert.True(t, hasCall(cmd.calls, "docker build --tag "+req.Tag), "expected a build, got %v", cmd.calls)
	assert.Equal(t, req.Tag, image)

	got, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, requirementsName))
	require.NoError(t, err)
	assert.Equal(t, "pandas==2.2.0\nrequests\n", string(got))
	// packages.txt must exist so the image's ONBUILD copy does not fail.
	assert.FileExists(t, filepath.Join(req.WorkDir, buildContextDir, packagesName))
	// The Dockerfile is `FROM <base>` and sits outside the context dir.
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, "FROM "+req.BaseImage+"\n", string(df))
}

func TestBuildWritesOSPackages(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Packages = []string{"libpq-dev", "build-essential"}

	image, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	// OS packages alone trigger the build layer, even with no extra deps.
	assert.True(t, hasCall(cmd.calls, "docker build --tag "+req.Tag), "expected a build, got %v", cmd.calls)
	assert.Equal(t, req.Tag, image)

	got, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, packagesName))
	require.NoError(t, err)
	assert.Equal(t, "libpq-dev\nbuild-essential\n", string(got))
	// requirements.txt is still written (no extra deps here) so its own ONBUILD
	// copy holds.
	assert.FileExists(t, filepath.Join(req.WorkDir, buildContextDir, requirementsName))
}

func TestBuildEmptyPackagesWritesEmptyFile(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"} // a build runs, but no OS packages

	_, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)

	// packages.txt must exist but stay empty when none are listed, so the
	// ONBUILD copy does not fail.
	got, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, packagesName))
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestBuildSkipsWhenNothingToInstall(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t) // no deps, no packages

	image, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	// Nothing to install: no build runs and the base image is returned as-is.
	assert.Empty(t, cmd.calls, "no build may run with nothing to install, got %v", cmd.calls)
	assert.Equal(t, req.BaseImage, image)
	// No context is assembled on the skip path.
	assert.NoDirExists(t, filepath.Join(req.WorkDir, buildContextDir))
}

func TestBuildAirflowOnlyDepsSkip(t *testing.T) {
	// The base image provides Airflow; a request listing only Airflow (any
	// extras/pin) needs no build layer.
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"apache-airflow==3.1.*", "apache-airflow[celery]"}

	image, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, cmd.calls, "airflow-only deps must not trigger a build, got %v", cmd.calls)
	assert.Equal(t, req.BaseImage, image)
}

func TestBuildFailedInstallReturnsNamedError(t *testing.T) {
	cmd := &fakeCmd{run: func(call string, _ localrt.Stdio) error {
		if strings.Contains(call, "build") {
			return errors.New("exit status 1")
		}
		return nil
	}}
	req := testRequest(t)
	req.Dependencies = []string{"nonexistent-package-xyz"}

	image, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies")
	assert.Empty(t, image)
}

func TestBuildStreamsOutput(t *testing.T) {
	cmd := &fakeCmd{run: func(_ string, s localrt.Stdio) error {
		_, _ = s.Out.Write([]byte("Step 1/1\n"))
		return nil
	}}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}

	var lines []localrt.LogLine
	cb := localrt.Callbacks{OnLine: func(l localrt.LogLine) { lines = append(lines, l) }}
	_, err := testBuilder(cmd).Build(context.Background(), req, cb)
	require.NoError(t, err)

	require.Len(t, lines, 1)
	assert.Equal(t, "build", lines[0].Component)
	assert.Equal(t, "Step 1/1", lines[0].Text)
	assert.Equal(t, fixedTime, lines[0].Time)
}

func TestRuntimeImage(t *testing.T) {
	ref, err := RuntimeImage("3.1-2")
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1-2", ref)

	// A floating tag passes through unchanged; the registry resolves it.
	ref, err = RuntimeImage("3.1")
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1", ref)

	_, err = RuntimeImage("2.9.3")
	assert.ErrorContains(t, err, "Airflow 3")

	_, err = RuntimeImage("")
	assert.ErrorContains(t, err, "no Airflow version")
}

func TestBuildPassesPlatform(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}
	req.Platform = "linux/amd64"

	_, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasCall(cmd.calls, "--platform linux/amd64"), "expected --platform in the build, got %v", cmd.calls)
}

func TestBuildOmitsPlatformWhenUnset(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"} // no platform: host build, exact command

	_, err := testBuilder(cmd).Build(context.Background(), req, localrt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasCall(cmd.calls, "--platform"), "host build must not pin a platform, got %v", cmd.calls)
}

func TestRuntimeDepsDropsAirflowOnly(t *testing.T) {
	got := runtimeDeps([]string{
		"apache-airflow==3.1.*",
		"apache-airflow[celery] >= 3",
		"APACHE_AIRFLOW==3",
		"apache-airflow-providers-postgres",
		"pandas",
		"requests>=2",
	})
	assert.Equal(t, []string{
		"apache-airflow-providers-postgres",
		"pandas",
		"requests>=2",
	}, got)
}
