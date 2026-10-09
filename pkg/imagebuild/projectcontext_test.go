package imagebuild

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

func callsStarting(calls []string, prefix string) []string {
	var out []string
	for _, c := range calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}
	return out
}

// A ProjectContext build installs the dependencies over the base, as every
// generated build does, then copies the project in with the project as the
// context, and drops the intermediate tag.
func TestBuildShipsTheProjectAsTheContextOfASecondStep(t *testing.T) {
	project := t.TempDir()
	cmd := &fakeCmd{}
	req := projectRequest(t, project)

	got, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p-abc123", got)

	builds := callsStarting(cmd.calls, "docker build")
	require.Len(t, builds, 2, "%v", cmd.calls)
	deps, proj := builds[0], builds[1]

	assert.Contains(t, deps, "--tag astro-deploy/p-abc123:latest-deps ")
	assert.Contains(t, deps, "--pull")
	assert.Contains(t, deps, "--secret id=netrc,env=NETRC")
	assert.True(t, strings.HasSuffix(deps, " "+filepath.Join(req.WorkDir, buildContextDir)), deps)

	projectDF := filepath.Join(req.WorkDir, projectDockerfileName)
	assert.Equal(t, "docker build --tag astro-deploy/p-abc123 --file "+projectDF+" --platform linux/amd64 "+project, proj,
		"no --pull (the base is local), no secrets (nothing runs), the project as the context")
	df, err := os.ReadFile(projectDF)
	require.NoError(t, err)
	assert.Equal(t, "FROM astro-deploy/p-abc123:latest-deps\nCOPY --chown=astro:0 . .\n", string(df))
	_, err = os.Stat(projectDF + ".dockerignore")
	require.NoError(t, err, "the ignore file sits beside the Dockerfile, where BuildKit reads it")

	assert.Equal(t, "docker image rm --no-prune astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
}

func TestBuildWithAProjectContextSkipsTheFastPath(t *testing.T) {
	cmd := &fakeCmd{}
	req := projectRequest(t, t.TempDir())
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.Tag, got)
	assert.Len(t, callsStarting(cmd.calls, "docker build"), 2)
}

// Without a ProjectContext the build is what it always was: one build over
// the two dependency files, and the fast path when there is nothing to
// install. Local Docker mode and Astro Desktop build this way.
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
	assert.Contains(t, cmd.calls[0], "--tag astro-deploy/p-abc123 ")
	_, err = os.Stat(filepath.Join(req.WorkDir, projectDockerfileName))
	assert.True(t, os.IsNotExist(err), "no project step")
}

func TestBuildWithAProjectContextOnPodman(t *testing.T) {
	project := t.TempDir()
	cmd := &fakeCmd{}
	req := projectRequest(t, project)
	req.Bin = "/opt/podman/bin/podman"

	_, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	builds := callsStarting(cmd.calls, "/opt/podman/bin/podman build")
	require.Len(t, builds, 2)
	ignore := filepath.Join(req.WorkDir, projectDockerfileName) + ".dockerignore"
	assert.Contains(t, builds[1], "--ignorefile "+ignore)
	assert.Equal(t, "/opt/podman/bin/podman untag astro-deploy/p-abc123:latest-deps astro-deploy/p-abc123:latest-deps", cmd.calls[len(cmd.calls)-1])
}

func TestBuildProjectStepFailureNamesItAndStillDropsTheIntermediateTag(t *testing.T) {
	project := t.TempDir()
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.Contains(call, projectDockerfileName) {
			return errors.New("exit status 1")
		}
		return nil
	}}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, project), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "copying the project into the image failed")
	assert.Contains(t, cmd.calls[len(cmd.calls)-1], "image rm --no-prune astro-deploy/p-abc123:latest-deps")
}

func TestBuildDependencyStepFailureStopsBeforeTheProject(t *testing.T) {
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.Contains(call, ":latest-deps") {
			return errors.New("exit status 1")
		}
		return nil
	}}
	_, err := testBuilder(cmd).BuildLocal(context.Background(), projectRequest(t, t.TempDir()), rt.Callbacks{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "installing the project's dependencies")
	assert.Len(t, cmd.calls, 2, "the failed build and the secret-mount probe, nothing after: %v", cmd.calls)
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

	got, err := ProjectIgnore(project, []string{"dags/"})
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	assert.Equal(t, []string{"secrets/", "!.env.example"}, lines[:2], "the project's rules, unchanged and first")
	assert.Equal(t, "dags/", lines[len(lines)-1], "the caller's excludes last")
	for _, rule := range []string{"**/.venv", "**/.env", ".astro", ".git", "/requirements.txt", "/packages.txt", "plugins/fix_local_executor_pickle.py"} {
		assert.Contains(t, lines, rule)
	}
}

func TestProjectIgnoreWithoutAProjectFile(t *testing.T) {
	got, err := ProjectIgnore(t.TempDir(), nil)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(got, projectIgnoreHeader), got)
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
