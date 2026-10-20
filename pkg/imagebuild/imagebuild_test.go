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

func TestBuildWritesTheInstallInputs(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas==2.2.0", "requests"}

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	// A build ran, tagged with the requested tag, and that tag is the image
	// returned to run.
	assert.True(t, hasCall(cmd.calls, "docker build --tag "+req.Tag), "expected a build, got %v", cmd.calls)
	assert.Equal(t, req.Tag, image)

	// A request naming no project carries its lists in a pyproject.toml of
	// its own, beside the helper that reads it.
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	helper, err := os.ReadFile(filepath.Join(ctxDir, filepath.FromSlash(InstallHelperPath)))
	require.NoError(t, err)
	assert.Equal(t, InstallHelper(), helper)
	assert.FileExists(t, filepath.Join(ctxDir, pyprojectName))
	reqs, pkgs, constraints := installLists(t, ctxDir)
	assert.Equal(t, "pandas==2.2.0\nrequests\n", reqs)
	assert.Empty(t, pkgs)
	assert.Empty(t, constraints)
}

// The Dockerfile starts FROM the base's -base tag, runs the helper in a stage
// of its own, writes the runtime's ONBUILD steps out over what it wrote, and
// sits outside the context dir.
func TestBuildWritesTheInstallDockerfile(t *testing.T) {
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	base := req.BaseImage + "-base"
	assert.Equal(t, "FROM "+base+" AS astro-install\n"+
		"USER root\n"+
		"COPY pyproject.toml uv.lock* .astro/install-find-links.txt* .astro/install-project.py /tmp/astro-project/\n"+
		"RUN python /tmp/astro-project/install-project.py --project /tmp/astro-project --out /tmp/astro-install\n"+
		"FROM "+base+"\n"+
		"COPY --from=astro-install /tmp/astro-install/packages.txt .\n"+
		"USER root\n"+
		"RUN if [ -x /usr/local/bin/install-system-packages ]; then /usr/local/bin/install-system-packages; "+
		"elif [ -s packages.txt ]; then apt-get update && cat packages.txt | tr '\\r\\n' '\\n' | sed -e 's/#.*//' | xargs apt-get install -y --no-install-recommends && apt-get clean && rm -rf /var/lib/apt/lists/*; fi\n"+
		"COPY --from=astro-install /tmp/astro-install/requirements.txt /tmp/astro-install/uv-lock-constraints.txt ./\n"+
		"RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false "+
		"if [ -x /usr/local/bin/install-python-dependencies ]; then /usr/local/bin/install-python-dependencies; "+
		"else pip install --no-cache-dir --root-user-action=ignore -r requirements.txt; fi\n"+
		"USER astro\n"+
		"COPY --chown=astro:0 . .\n", string(df))
	assert.NoFileExists(t, filepath.Join(req.WorkDir, buildContextDir, dockerfileName))
}

// A request naming its project installs from the project's own pyproject.toml
// and uv.lock, copied into the context as they are.
func TestBuildCopiesTheProjectsManifestAndLock(t *testing.T) {
	project := t.TempDir()
	pyproject, lock := lockProject([]string{"pandas"}, "[[package]]\nname = \"pandas\"\nversion = \"2.2.3\"\nsource = { registry = \"https://pypi.org/simple\" }\n")
	writeTree(t, project, map[string]string{"pyproject.toml": pyproject, "uv.lock": lock})
	req := testRequest(t)
	req.ManifestDir = project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	for name, want := range map[string]string{pyprojectName: pyproject, uvLockName: lock} {
		got, err := os.ReadFile(filepath.Join(ctxDir, name))
		require.NoError(t, err)
		assert.Equal(t, want, string(got), name)
	}
	reqs, _, constraints := installLists(t, ctxDir)
	assert.Equal(t, "-c "+lockConstraintsName+"\npandas\n", reqs)
	assert.Contains(t, constraints, "\npandas==2.2.3\n")
}

// A project with no uv.lock installs its dependencies unpinned, as before.
func TestBuildWithoutALockInstallsUnpinned(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{"pyproject.toml": "[project]\ndependencies = ['pandas']\n"})
	req := testRequest(t)
	req.ManifestDir = project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	assert.NoFileExists(t, filepath.Join(ctxDir, uvLockName))
	reqs, _, constraints := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs)
	assert.Empty(t, constraints)
}

// Find-links pages lead the generated requirements.txt, one --find-links
// line each, before the dependencies they serve, from a request naming no
// project too: its pyproject.toml routes each page's package to its index.
func TestBuildWritesFindLinks(t *testing.T) {
	req := testRequest(t)
	req.Dependencies = []string{"private-lib", "pandas"}
	// Passed through as they are, whatever their shape.
	req.FindLinks = []string{"https://pypi.acme.example/simple/private-lib/", "https://files.acme.example/wheels/index.html", "./vendor"}

	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	reqs, _, _ := installLists(t, filepath.Join(req.WorkDir, buildContextDir))
	assert.Equal(t, "--find-links https://pypi.acme.example/simple/private-lib/\n"+
		"--find-links https://files.acme.example/wheels/index.html\n"+
		"--find-links ./vendor\nprivate-lib\npandas\n", reqs)
}

// Local Docker mode builds in the same WorkDir every start, so the context is
// cleared each time: a uv.lock the project has since deleted is not pinned
// from.
func TestBuildForgetsALockTheProjectDeleted(t *testing.T) {
	project := t.TempDir()
	pyproject, lock := lockProject([]string{"pandas"}, "[[package]]\nname = \"pandas\"\nversion = \"2.2.3\"\nsource = { registry = \"https://pypi.org/simple\" }\n")
	writeTree(t, project, map[string]string{"pyproject.toml": pyproject, "uv.lock": lock})
	req := testRequest(t)
	req.ManifestDir = project
	req.Dependencies = []string{"pandas"}
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)

	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	_, _, constraints := installLists(t, ctxDir)
	require.Contains(t, constraints, "pandas==2.2.3")

	require.NoError(t, os.Remove(filepath.Join(project, "uv.lock")))
	_, err = testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.NoFileExists(t, filepath.Join(ctxDir, uvLockName))
	reqs, _, constraints := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs)
	assert.Empty(t, constraints)
}

// A shipped build from a caller that sets ProjectFiles but predates
// ManifestDir installs from the project's own pyproject.toml, and ships it as
// it is, not a stand-in written from the request's lists.
func TestBuildWithoutManifestDirReadsTheShippedProject(t *testing.T) {
	project := t.TempDir()
	const pyproject = "[project]\nname = 'p'\ndependencies = ['pandas']\n\n[tool.ruff]\nline-length = 100\n"
	writeTree(t, project, map[string]string{"pyproject.toml": pyproject})
	req := testRequest(t)
	req.ProjectFiles = project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	got, err := os.ReadFile(filepath.Join(ctxDir, pyprojectName))
	require.NoError(t, err)
	assert.Equal(t, pyproject, string(got))
	reqs, _, _ := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs)
}

// The -base suffix is added once: a base already on a -base tag is not
// suffixed again, and GeneratedDockerfile starts from the same image Build
// does.
func TestBaseOf(t *testing.T) {
	assert.Equal(t, RuntimeImageRepo+":3.1-5-base", baseOf(RuntimeImageRepo+":3.1-5"))
	assert.Equal(t, "quay.io/astronomer/astro-runtime:12.6.0-python-3.11-base", baseOf("quay.io/astronomer/astro-runtime:12.6.0-python-3.11"))
	assert.Equal(t, RuntimeImageRepo+":3.1-base", baseOf(RuntimeImageRepo+":3.1-base"))
	assert.Equal(t, InstallDockerfile(RuntimeImageRepo+":3.1-base"), GeneratedDockerfile(RuntimeImageRepo+":3.1"))
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

	reqs, pkgs, _ := installLists(t, filepath.Join(req.WorkDir, buildContextDir))
	assert.Equal(t, "libpq-dev\nbuild-essential\n", pkgs)
	assert.Equal(t, "\n", reqs, "written with nothing in it, so the install's COPY finds it")
}

// A git direct reference in a request naming no project still brings git in.
func TestBuildAddsGitForAGitReference(t *testing.T) {
	req := testRequest(t)
	req.Dependencies = []string{"airflow-blueprint @ git+https://github.com/astronomer/blueprint@6f06313"}
	req.Packages = []string{"libpq-dev"}
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	_, pkgs, _ := installLists(t, filepath.Join(req.WorkDir, buildContextDir))
	assert.Equal(t, "libpq-dev\ngit\n", pkgs)
	assert.Equal(t, []string{"libpq-dev"}, req.Packages, "the caller's list is not modified")
}

func TestBuildSkipsWhenNothingToInstall(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t) // no deps, no packages

	image, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	// Nothing to install: no build runs and the base's -base image is
	// returned as-is, the one a build would have started FROM.
	assert.Empty(t, cmd.calls, "no build may run with nothing to install, got %v", cmd.calls)
	assert.Equal(t, req.BaseImage+"-base", image)
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
	assert.Equal(t, req.BaseImage+"-base", image)
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
		assert.Contains(t, err.Error(), "Airflow pin", "pin %q", pin)
		assert.Contains(t, err.Error(), "apache-airflow==", "pin %q", pin)
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
		"apache-airflow-task-sdk>=1.1",
		"pandas",
		"requests>=2",
	})
	assert.Equal(t, []string{
		"apache-airflow-providers-postgres",
		"apache-airflow-task-sdk>=1.1",
		"pandas",
		"requests>=2",
	}, got)
}

// apache-airflow-core is dropped too. The install script does not refuse it,
// so a core requirement the image's exact core does not satisfy got as far as
// uv and failed there, with a resolver trace that named neither the base
// image nor the manifest.
func TestRuntimeDepsDropsAirflowCore(t *testing.T) {
	got := runtimeDeps([]string{
		"apache-airflow-core==3.2.*",
		"Apache_Airflow_Core[otel]==3.3.2 ; sys_platform == 'linux'",
		"apache-airflow-providers-standard",
	})
	assert.Equal(t, []string{"apache-airflow-providers-standard"}, got)
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
		{
			// A mirror is an Astro Runtime image (airflowrt.IsAstroRuntimeRef),
			// but not one a pull can be forced on: it may be in a registry this
			// daemon is not logged into, or loaded with no registry at all.
			name: "declared, a mirrored runtime",
			from: "FROM registry.corp.example/astronomer/astro-runtime:12.1.0\n",
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
	assert.ErrorIs(t, err, ErrDockerfileBuild)
	assert.Equal(t, "building the project's Dockerfile failed; see the build output above: exit status 1", err.Error())

	req := dockerfileRequest(t)
	req.Dockerfile, req.Context = "", ""
	_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDockerfileBuild, "a generated build's failure is not the project's Dockerfile")
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

// Format reaches podman as --format, and an empty one passes none, so a
// docker build, whose CLI has no such flag, keeps its exact command.
func TestBuildPassesFormatOnlyWhenSet(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)
	req.Bin = "podman"
	req.Format = "docker"
	req.Platform = "linux/amd64"

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.Equal(t, "podman build --tag "+req.Tag+" --file "+req.Dockerfile+" --format docker --platform linux/amd64 "+req.Context, cmd.calls[0])

	cmd = &fakeCmd{}
	req.Format = ""
	_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.False(t, hasCall(cmd.calls, "--format"), "no Format, no --format, got %v", cmd.calls)
}

// Several platforms are one --platform value, as docker takes them.
func TestBuildPassesSeveralPlatformsAsOneValue(t *testing.T) {
	cmd := &fakeCmd{}
	req := dockerfileRequest(t)
	req.Platform = "linux/amd64,linux/arm64"

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.True(t, hasCall(cmd.calls, "--platform linux/amd64,linux/arm64 "), "got %v", cmd.calls)
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

// A generated build passes the netrc secret, the one the runtime image's
// install step mounts, and drops any other, which no step of the build reads.
func TestBuildGeneratedModePassesOnlyTheNetrcSecret(t *testing.T) {
	cmd := &fakeCmd{}
	req := testRequest(t)
	req.Dependencies = []string{"pandas"}
	req.Secrets = []string{"id=pypi,src=/tmp/pypi.txt", "id=netrc,env=NETRC_CONTENT"}

	if _, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{}); err != nil {
		t.Fatal(err)
	}
	if !hasCall(cmd.calls, "--secret id=netrc,env=NETRC_CONTENT") {
		t.Errorf("calls = %v, want the netrc secret on a generated build", cmd.calls)
	}
	if hasCall(cmd.calls, "id=pypi") {
		t.Errorf("calls = %v, want no other secret on a generated build", cmd.calls)
	}
}

// The netrc secret reaches the install on every runtime, because the mount is
// the generated Dockerfile's own step rather than the base's ONBUILD trigger,
// which runtimes before the mount lack. So a failed build has nothing to ask
// the base about: it runs the build and nothing else.
func TestGeneratedBuildMountsNetrcWhateverTheBase(t *testing.T) {
	cmd := &fakeCmd{run: func(string, rt.Stdio) error { return errors.New("exit status 1") }}
	req := testRequest(t)
	req.Dependencies = []string{"example-lib @ git+https://github.com/example-org/example-lib.git"}
	req.Secrets = []string{"id=netrc,env=NETRC_CONTENT"}

	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.Error(t, err)
	require.Len(t, cmd.calls, 1, "the build alone, got %v", cmd.calls)
	assert.Contains(t, cmd.calls[0], "--secret id=netrc,env=NETRC_CONTENT")
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Contains(t, string(df), "RUN --mount=type=secret,id=netrc,target=/root/.netrc,required=false ")
}
