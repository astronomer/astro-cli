package imagebuild

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

func projectRequest(t *testing.T, project string) Request {
	t.Helper()
	return Request{
		WorkDir:        t.TempDir(),
		BaseImage:      "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:            "astro-deploy/p-abc123",
		ProjectContext: project,
		Platform:       "linux/amd64",
		Secrets:        []string{"id=netrc,env=NETRC"},
		Bin:            "docker",
	}
}

// engine is a fakeCmd hook answering the probes as an engine would: what
// `--version` prints, whether `buildx version` answers, and the help of
// `build`. fail, when set, fails the calls it matches.
func engine(version string, buildx bool, buildHelp string, fail func(call string) error) func(string, rt.Stdio) error {
	return func(call string, s rt.Stdio) error {
		_, args, _ := strings.Cut(call, " ")
		switch args {
		case "--version":
			_, _ = io.WriteString(s.Out, version)
			return nil
		case "buildx version":
			if !buildx {
				return errors.New("docker: 'buildx' is not a docker command")
			}
			return nil
		case "build --help":
			_, _ = io.WriteString(s.Out, buildHelp)
			return nil
		}
		if fail != nil {
			return fail(call)
		}
		return nil
	}
}

const dockerVersion = "Docker version 29.4.0, build 1234567"

func callsContaining(calls []string, sub string) []string {
	var out []string
	for _, c := range calls {
		if strings.Contains(c, sub) {
			out = append(out, c)
		}
	}
	return out
}

// A ProjectContext build installs the dependencies over the base, as every
// generated build does, then copies the project in with BuildKit (buildx),
// the project as the context, and drops the intermediate tag.
func TestBuildShipsTheProjectAsTheContextOfASecondStep(t *testing.T) {
	project := t.TempDir()
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	req := projectRequest(t, project)

	got, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p-abc123", got)

	assert.Equal(t, []string{"docker --version", "docker buildx version"}, cmd.calls[:2], "the engine is asked before anything is built")
	deps := callsContaining(cmd.calls, "docker build --tag")
	require.Len(t, deps, 1, "%v", cmd.calls)
	assert.Contains(t, deps[0], "--tag astro-deploy/p-abc123:latest-deps ")
	assert.Contains(t, deps[0], "--pull")
	assert.Contains(t, deps[0], "--secret id=netrc,env=NETRC")
	assert.True(t, strings.HasSuffix(deps[0], " "+filepath.Join(req.WorkDir, buildContextDir)), deps[0])

	projectDF := filepath.Join(req.WorkDir, projectDockerfileName)
	proj := callsContaining(cmd.calls, "buildx build")
	require.Len(t, proj, 1, "%v", cmd.calls)
	assert.Equal(t, "docker buildx build --load --tag astro-deploy/p-abc123 --file "+projectDF+" --platform linux/amd64 "+project, proj[0],
		"BuildKit only, no --pull (the base is local), no secrets (nothing runs), the project as the context")
	df, err := os.ReadFile(projectDF)
	require.NoError(t, err)
	assert.Equal(t, "FROM astro-deploy/p-abc123:latest-deps\nCOPY --chown=astro:0 . .\n", string(df))
	_, err = os.Stat(projectDF + ".dockerignore")
	require.NoError(t, err, "the ignore file sits beside the Dockerfile, where BuildKit reads it")

	assert.Equal(t, "docker image rm --no-prune astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
}

// The step that copies the project runs with DOCKER_BUILDKIT=1 in its
// environment, as well as through buildx.
func TestBuildProjectStepAsksForBuildKit(t *testing.T) {
	var env []string
	cmd := &envCmd{fakeCmd: fakeCmd{run: engine(dockerVersion, true, "", nil)}, onBuildx: func(e []string) { env = e }}
	_, err := New(cmd, func() time.Time { return fixedTime }).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.NoError(t, err)
	assert.Contains(t, env, "DOCKER_BUILDKIT=1")
}

// envCmd is fakeCmd that hands the environment of a buildx call to onBuildx.
type envCmd struct {
	fakeCmd
	onBuildx func([]string)
}

func (e *envCmd) Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	if len(args) > 0 && args[0] == "buildx" && len(args) > 1 && args[1] == "build" {
		e.onBuildx(env)
	}
	return e.fakeCmd.Run(ctx, env, s, name, args...)
}

// Without buildx, `docker build` may be the legacy builder, which ignores
// <Dockerfile>.dockerignore and would copy .env and the rest into the image.
// The build is refused before anything is built.
func TestBuildProjectRefusesDockerWithoutBuildx(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, false, "", nil)}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.ErrorIs(t, err, errNoProjectBuilder)
	assert.Contains(t, err.Error(), "buildx")
	assert.Empty(t, callsContaining(cmd.calls, "build --"), "nothing is built: %v", cmd.calls)
}

func TestBuildWithAProjectContextSkipsTheFastPath(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", nil)}
	req := projectRequest(t, t.TempDir())
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, got)
	assert.Len(t, callsContaining(cmd.calls, "build --"), 2)
}

// Without a ProjectContext the build is what it always was: one build over
// the two dependency files, no probes, and the fast path when there is
// nothing to install. Local Docker mode and Astro Desktop build this way.
func TestBuildWithoutAProjectContextIsUnchanged(t *testing.T) {
	req := projectRequest(t, "")
	cmd := &fakeCmd{}
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.BaseImage, got)
	assert.Empty(t, cmd.calls)

	req.Dependencies = []string{"pandas"}
	_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], "docker build --tag astro-deploy/p-abc123 ")
	_, err = os.Stat(filepath.Join(req.WorkDir, projectDockerfileName))
	assert.True(t, os.IsNotExist(err), "no project step")
}

// Podman, called by its name or through the podman-docker shim, is told the
// ignore file with --ignorefile, and drops the intermediate name with untag.
func TestBuildWithAProjectContextOnPodman(t *testing.T) {
	for name, bin := range map[string]string{"podman": "/opt/podman/bin/podman", "the podman-docker shim": "docker"} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			cmd := &fakeCmd{run: engine("podman version 5.8.2", false, "      --ignorefile string   path to an alternate .dockerignore file", nil)}
			req := projectRequest(t, project)
			req.Bin = bin

			_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
			require.NoError(t, err)

			ignore := filepath.Join(req.WorkDir, projectDockerfileName) + ".dockerignore"
			proj := callsContaining(cmd.calls, "--ignorefile")
			require.Len(t, proj, 1, "%v", cmd.calls)
			assert.True(t, strings.HasPrefix(proj[0], bin+" build --tag astro-deploy/p-abc123 --file "), proj[0])
			assert.Contains(t, proj[0], "--ignorefile "+ignore)
			assert.Empty(t, callsContaining(cmd.calls, "buildx"))
			assert.Equal(t, bin+" untag astro-deploy/p-abc123:latest-deps astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
		})
	}
}

// A podman whose build has no --ignorefile cannot be told the ignore file, so
// the build is refused before anything is built.
func TestBuildProjectRefusesPodmanWithoutIgnorefile(t *testing.T) {
	cmd := &fakeCmd{run: engine("podman version 2.0.0", false, "      --file string   Dockerfile", nil)}
	req := projectRequest(t, t.TempDir())
	req.Bin = "podman"
	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, errNoProjectBuilder)
	assert.Contains(t, err.Error(), "--ignorefile")
	assert.Empty(t, callsContaining(cmd.calls, "build --tag"))
}

func TestBuildProjectStepFailureNamesItAndStillDropsTheIntermediateTag(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, projectDockerfileName) {
			return errors.New("exit status 1")
		}
		return nil
	})}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copying the project into the image failed")
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune astro-deploy/p-abc123:latest-deps")
}

// An interrupted build still drops the intermediate tag: the removal runs on
// a context the build's cancellation does not reach.
func TestBuildProjectDropsTheIntermediateTagWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var untagCtxErr error
	cmd := &ctxCmd{fakeCmd: fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, projectDockerfileName) {
			cancel()
			return context.Canceled
		}
		return nil
	})}, onUntag: func(c context.Context) { untagCtxErr = c.Err() }}

	_, err := New(cmd, func() time.Time { return fixedTime }).BuildLocal(ctx, projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune astro-deploy/p-abc123:latest-deps")
	assert.NoError(t, untagCtxErr, "the removal's context is not the canceled one")
}

// ctxCmd is fakeCmd that hands the context of the untag call to onUntag.
type ctxCmd struct {
	fakeCmd
	onUntag func(context.Context)
}

func (c *ctxCmd) Run(ctx context.Context, env []string, s rt.Stdio, name string, args ...string) error {
	if len(args) > 1 && args[0] == "image" && args[1] == "rm" {
		c.onUntag(ctx)
	}
	return c.fakeCmd.Run(ctx, env, s, name, args...)
}

func TestBuildDependencyStepFailureStopsBeforeTheProject(t *testing.T) {
	cmd := &fakeCmd{run: engine(dockerVersion, true, "", func(call string) error {
		if strings.Contains(call, ":latest-deps") {
			return errors.New("exit status 1")
		}
		return nil
	})}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installing the project's dependencies")
	assert.Empty(t, callsContaining(cmd.calls, "buildx build"))
	assert.Empty(t, callsContaining(cmd.calls, "image rm"))
}

// A declared Dockerfile builds the project as its context already.
func TestBuildDockerfileModeIgnoresTheProjectContext(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, "Dockerfile"), []byte("FROM astrocrpublic.azurecr.io/runtime:3.1\n"), 0o600))
	req := projectRequest(t, project)
	req.Dockerfile, req.Context = filepath.Join(project, "Dockerfile"), project
	cmd := &fakeCmd{}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.NotContains(t, cmd.calls[0], "-deps")
}

func TestProjectIgnoreKeepsTheProjectsRulesAndPutsTheCLIsAfter(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(project, ".dockerignore"), []byte("secrets/\n!.env.example"), 0o600))

	got, err := ProjectIgnore(project, []string{"dags"})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	assert.Equal(t, []string{"secrets/", "!.env.example"}, lines[:2], "the project's rules, unchanged and first")
	assert.Equal(t, "dags", lines[len(lines)-1], "the caller's excludes last")
	// 1.x's default .dockerignore, then v2's per-machine files.
	for _, rule := range []string{
		"astro", ".git", "airflow_settings.yaml", "logs", "airflow.db", "airflow.cfg",
		".astro", "**/.venv", "**/.env", "**/.env.*", "**/.envrc", "**/__pycache__", "**/*.pyc",
		"plugins/fix_local_executor_pickle.py", "requirements.txt", "packages.txt",
	} {
		assert.Contains(t, lines, rule)
	}
	for _, line := range lines {
		assert.False(t, strings.HasPrefix(line, "/"), "%q: no leading slash, which buildah and Docker may read differently", line)
	}
}

func TestProjectIgnoreWithoutAProjectFile(t *testing.T) {
	got, err := ProjectIgnore(t.TempDir(), nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, projectIgnoreHeader), got)
}

func TestShipProject(t *testing.T) {
	req := Request{}
	ShipProject(&req, "/p", true)
	assert.Equal(t, "/p", req.ProjectContext)
	assert.Empty(t, req.ProjectExcludes)

	ShipProject(&req, "/p", false)
	assert.Equal(t, []string{"dags"}, req.ProjectExcludes)

	declared := Request{Dockerfile: "/p/Dockerfile", Context: "/p"}
	ShipProject(&declared, "/p", false)
	assert.Empty(t, declared.ProjectContext, "a declared Dockerfile's context is the project already")
}

func TestDepsTag(t *testing.T) {
	for tag, want := range map[string]string{
		"astro-deploy/p-abc123":            "astro-deploy/p-abc123:latest-deps",
		"astro-package/p:src-1234567":      "astro-package/p:src-1234567-deps",
		"localhost:5000/team/p":            "localhost:5000/team/p:latest-deps",
		"localhost:5000/team/p:3.1-2-abcd": "localhost:5000/team/p:3.1-2-abcd-deps",
	} {
		assert.Equal(t, want, depsTag(tag), tag)
	}
}
