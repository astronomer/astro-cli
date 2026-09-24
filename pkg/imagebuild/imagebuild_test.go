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

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// fakeCmd is a Commander that never touches a real daemon. Each call is
// recorded as "name arg arg..."; a hook drives the response and can read the
// wired stdio.
type fakeCmd struct {
	calls []string
	run   func(call string, s rt.Stdio) error
}

func (f *fakeCmd) Run(_ context.Context, _ []string, s rt.Stdio, name string, args ...string) error {
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

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
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

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
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

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
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

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
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

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, cmd.calls, "airflow-only deps must not trigger a build, got %v", cmd.calls)
	assert.Equal(t, req.BaseImage, image)
}

func TestBuildFailedInstallReturnsNamedError(t *testing.T) {
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.Contains(call, "build") {
			return errors.New("exit status 1")
		}
		return nil
	}}
	req := testRequest(t)
	req.Dependencies = []string{"nonexistent-package-xyz"}

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dependencies")
	assert.Empty(t, image)
}

func TestBuildStreamsOutput(t *testing.T) {
	cmd := &fakeCmd{run: func(_ string, s rt.Stdio) error {
		_, _ = s.Out.Write([]byte("Step 1/1\n"))
		return nil
	}}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}

	var lines []rt.LogLine
	cb := rt.Callbacks{OnLine: func(l rt.LogLine) { lines = append(lines, l) }}
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

// A manifest pin builds FROM its series: runtime:3.1.2 and runtime:3 are not
// published tags.
func TestRuntimeImageBuildsFromThePinsSeries(t *testing.T) {
	for _, tc := range []struct{ pin, want string }{
		{"3.1.2", RuntimeImageRepo + ":3.1"},
		{"3.1", RuntimeImageRepo + ":3.1"},
		{" 3.1.2 ", RuntimeImageRepo + ":3.1"},
		{"3.10.1", RuntimeImageRepo + ":3.10"},
	} {
		ref, err := RuntimeImage(tc.pin)
		require.NoError(t, err, "pin %q", tc.pin)
		assert.Equal(t, tc.want, ref, "pin %q", tc.pin)
	}
}

// A bare major names no series, and the refusal names the manifest's pin
// rather than leaving a registry error to explain it.
func TestRuntimeImageRefusesABareMajorNamingThePin(t *testing.T) {
	for _, pin := range []string{"3", " 3 ", "3."} {
		_, err := RuntimeImage(pin)
		require.Error(t, err, "pin %q", pin)
		assert.Contains(t, err.Error(), "airflow pin", "pin %q", pin)
		assert.Contains(t, err.Error(), "pyproject.toml", "pin %q", pin)
	}
}

func TestAirflowSeries(t *testing.T) {
	for pin, want := range map[string]string{
		"3.1.2":   "3.1",
		"3.1":     "3.1",
		"\t3.1.2": "3.1",
		"2.9.3":   "2.9",
		"3.1-2":   "3.1-2",
	} {
		got, ok := AirflowSeries(pin)
		assert.True(t, ok, "pin %q", pin)
		assert.Equal(t, want, got, "pin %q", pin)
	}
	for _, pin := range []string{"", "3", "3.", ".1", "  "} {
		got, ok := AirflowSeries(pin)
		assert.False(t, ok, "pin %q", pin)
		assert.Empty(t, got, "pin %q", pin)
	}
}

func TestBuildPassesPlatform(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}
	req.Platform = "linux/amd64"

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasCall(cmd.calls, "--platform linux/amd64"), "expected --platform in the build, got %v", cmd.calls)
}

func TestBuildOmitsPlatformWhenUnset(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"} // no platform: host build, exact command

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
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

// --pull is how a floating base tag picks up a new patch, so a generated build
// always wants it. A declared Dockerfile's FROM belongs to the user, and it may
// name a locally built image or a registry this daemon cannot reach — where
// forcing a pull fails a build that plain `docker build` completes. v1 drew the
// line at "any FROM that is not an Astro base", and a project moving to v2 has
// to keep the behavior it had.
func TestPullOnlyWhenEveryBaseComesFromAstro(t *testing.T) {
	for _, tc := range []struct {
		name string
		from string // "" builds the generated Dockerfile instead of a declared one
		want bool
	}{
		{name: "generated build, base tag floats", want: true},
		{name: "declared, astro registry", from: "FROM astrocrpublic.azurecr.io/runtime:3.1\n", want: true},
		{name: "declared, quay", from: "FROM quay.io/astronomer/astro-runtime:12\n", want: true},
		{name: "declared, an image only this machine has", from: "FROM my-own-base\n", want: false},
		{
			// Any stage, not the last one: a builder stage on an unreachable
			// image fails the build just as hard as a final one would.
			name: "declared, astro final stage over a foreign builder",
			from: "FROM golang:1.26 AS builder\nFROM astrocrpublic.azurecr.io/runtime:3.1\n",
			want: false,
		},
		{
			// docker splits on any whitespace. Read as "no FROM at all", this
			// base looks generated and gets the pull that breaks it.
			name: "declared with a tab, still the user's own base",
			from: "FROM\tmy-own-base\n",
			want: false,
		},
		{
			// The flag is not the image. Reading it as one drops base freshness
			// from a Dockerfile that is on an Astro base after all.
			name: "declared, astro base behind a platform flag",
			from: "FROM --platform=$BUILDPLATFORM astrocrpublic.azurecr.io/runtime:3.1\n",
			want: true,
		},
		{
			// Somebody else's registry that happens to share our prefix.
			name: "declared, a registry that only looks like ours",
			from: "FROM quay.io/astronomerfake/runtime:1\n",
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &fakeCmd{}
			var req Request
			if tc.from == "" {
				req = testRequest(t)
				req.Dependencies = []string{"pandas"}
			} else {
				req = dockerfileRequest(t)
				require.NoError(t, os.WriteFile(req.Dockerfile, []byte(tc.from), 0o600))
			}

			_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
			require.NoError(t, err)
			assert.Equal(t, tc.want, hasCall(cmd.calls, "--pull"),
				"wanted --pull=%v, got %v", tc.want, cmd.calls)
		})
	}
}

// dockerfileRequest is a tier-3 build: the project supplied its own Dockerfile,
// so the file and the project context replace the generated pair. Dependencies
// are set deliberately — a manifest carries them whichever tier it chose, and
// this mode has to ignore them rather than act on them.
func dockerfileRequest(t *testing.T) Request {
	t.Helper()
	project := t.TempDir()
	df := filepath.Join(project, "Dockerfile")
	require.NoError(t, os.WriteFile(df, []byte("FROM my-own-base\nRUN echo hi\n"), 0o600))
	return Request{
		WorkDir:      t.TempDir(),
		Dockerfile:   df,
		Context:      project,
		Tag:          "astro-local/my-project",
		Bin:          "docker",
		Dependencies: []string{"pandas"},
		Packages:     []string{"libaio"},
	}
}

func TestBuildDockerfileModeUsesTheProjectFileAndContext(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)

	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, got)
	assert.True(t, hasCall(cmd.calls, "--file "+req.Dockerfile),
		"expected the project's Dockerfile, got %v", cmd.calls)
	assert.True(t, hasCall(cmd.calls, req.Context),
		"expected the project as build context, got %v", cmd.calls)
}

// The generated files are how the runtime image's ONBUILD triggers install, and
// a Dockerfile project installs its own way. Writing them anyway would put a
// requirements.txt into the user's build context that their own COPY could pick
// up.
func TestBuildDockerfileModeWritesNothing(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	for _, name := range []string{requirementsName, packagesName, dockerfileName} {
		_, err := os.Stat(filepath.Join(req.WorkDir, name))
		assert.True(t, os.IsNotExist(err), "%s must not be generated in Dockerfile mode", name)
		_, err = os.Stat(filepath.Join(req.WorkDir, buildContextDir, name))
		assert.True(t, os.IsNotExist(err), "%s must not be generated in Dockerfile mode", name)
	}
	// And nothing was written into the project itself: a compiler does not
	// mutate its source.
	entries, err := os.ReadDir(req.Context)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "Dockerfile mode wrote into the project: %v", entries)
}

// The no-op fast path is keyed on having nothing to install, which is never true
// of a file whose steps we cannot read.
func TestBuildDockerfileModeBuildsEvenWithNoDependencies(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)
	req.Dependencies = nil
	req.Packages = nil

	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, got)
	assert.True(t, hasCall(cmd.calls, "build"), "expected a build, got %v", cmd.calls)
}

// A failure has to name whose file broke, because the two modes send the reader
// to different places.
func TestBuildDockerfileModeFailureNamesTheProjectFile(t *testing.T) {
	cmd := &fakeCmd{run: func(string, rt.Stdio) error { return errors.New("exit status 1") }}

	_, err := testBuilder(cmd).Build(context.Background(), dockerfileRequest(t), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "building the project's Dockerfile failed")
	assert.NotContains(t, err.Error(), "installing the project's dependencies")
}

func TestBuildDockerfileModeStillPinsPlatform(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)
	req.Platform = "linux/amd64"

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasCall(cmd.calls, "--platform linux/amd64"),
		"a deploy build pins the platform in either mode, got %v", cmd.calls)
}

// A declared Dockerfile that names nothing is refused, before docker runs.
//
// Checked here rather than in each caller because this is where they meet:
// localdocker reaches Build through the rt.ImageBuilder seam, and deploy and
// package call it directly. An earlier version of this guard lived in
// localdocker, which left the other two paths to fail inside a build with output
// that never mentions the pyproject.toml key the user has to fix.
//
// The directory rows are the ones that get here by accident: pkg/manifest
// validates the declared path with filepath.IsLocal, which is lexical and says
// yes to ".", so a check that only asked whether the path EXISTS would pass the
// project directory and hand `docker build -f` a directory.
func TestBuildRefusesADeclaredDockerfileItCannotRead(t *testing.T) {
	for _, tc := range []struct {
		name    string
		setup   func(t *testing.T, dir string) string // returns the Dockerfile path to declare
		wantMsg string
	}{
		{
			name:    "missing",
			setup:   func(_ *testing.T, dir string) string { return filepath.Join(dir, "docker", "Dockerfile") },
			wantMsg: "could not be read",
		},
		{
			name:    "the project directory itself",
			setup:   func(_ *testing.T, dir string) string { return dir },
			wantMsg: "is not a file",
		},
		{
			name: "some other directory",
			setup: func(t *testing.T, dir string) string {
				t.Helper()
				sub := filepath.Join(dir, "dags")
				if err := os.MkdirAll(sub, 0o750); err != nil {
					t.Fatal(err)
				}
				return sub
			},
			wantMsg: "is not a file",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := &fakeCmd{}
			req := testRequest(t)
			dir := t.TempDir()
			req.Dockerfile = tc.setup(t, dir)
			req.Context = dir

			_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
			if err == nil {
				t.Fatal("want an error naming the declared Dockerfile")
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error = %q, want it to contain %q", err, tc.wantMsg)
			}
			if !strings.Contains(err.Error(), req.Dockerfile) {
				t.Errorf("error = %q, want it to name the path %q, which is the thing to fix", err, req.Dockerfile)
			}
			if len(cmd.calls) != 0 {
				t.Errorf("docker was invoked (%v); a failure knowable without asking docker should not cost a build", cmd.calls)
			}
		})
	}
}

// And a declared Dockerfile that IS there builds from it, with the project as
// the context so a COPY of anything beside the file still resolves.
func TestBuildUsesADeclaredDockerfile(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	dir := t.TempDir()
	sub := filepath.Join(dir, "docker")
	if err := os.MkdirAll(sub, 0o750); err != nil {
		t.Fatal(err)
	}
	dockerfile := filepath.Join(sub, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM my-own-base\nRUN echo hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req.Dockerfile = dockerfile
	req.Context = dir

	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	if err != nil {
		t.Fatal(err)
	}
	if got != req.Tag {
		t.Errorf("Build = %q, want the built tag %q", got, req.Tag)
	}
	if !hasCall(cmd.calls, "--file "+dockerfile) {
		t.Errorf("calls = %v, want the declared file passed to --file", cmd.calls)
	}
	if !hasCall(cmd.calls, " "+dir) {
		t.Errorf("calls = %v, want the project as the build context", cmd.calls)
	}
}

// Secrets reach docker build as --secret, one flag per spec, in order.
//
// The SPEC is forwarded, never a value: docker reads the secret itself from the
// src file or the named env var, which is why these strings are safe on a
// command line and in the build log they end up in.
func TestBuildForwardsSecretsToDocker(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	dir := t.TempDir()
	dockerfile := filepath.Join(dir, "Dockerfile")
	if err := os.WriteFile(dockerfile, []byte("FROM my-own-base\nRUN --mount=type=secret,id=pypi true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	req.Dockerfile, req.Context = dockerfile, dir
	req.Secrets = []string{"id=pypi,src=/tmp/pypi.txt", "id=other,env=OTHER"}

	if _, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(cmd.calls, "--secret id=pypi,src=/tmp/pypi.txt") {
		t.Errorf("calls = %v, want the first secret forwarded", cmd.calls)
	}
	if !hasCall(cmd.calls, "--secret id=other,env=OTHER") {
		t.Errorf("calls = %v, want the second secret forwarded", cmd.calls)
	}
	if strings.Count(strings.Join(cmd.calls, " "), "--secret") != 2 {
		t.Errorf("calls = %v, want exactly one --secret per spec", cmd.calls)
	}
}

// A generated build passes no secret, because there is nothing of the project's
// to mount one into: its Dockerfile is `FROM <base>` and the install happens in
// the runtime image's own ONBUILD triggers. Callers refuse the combination, and
// this pins that nothing here smuggles it through anyway.
func TestBuildGeneratedModeIgnoresSecrets(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}
	req.Secrets = []string{"id=pypi,src=/tmp/pypi.txt"}

	if _, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	if hasCall(cmd.calls, "--secret") {
		t.Errorf("calls = %v, want no --secret on a generated build", cmd.calls)
	}
}
