package imagebuild

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// declaredProject writes a project with a Dockerfile at docker/Dockerfile and
// returns its root.
func declaredProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docker"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker", "Dockerfile"),
		[]byte("FROM astrocrpublic.azurecr.io/runtime:3.1-2\nRUN --mount=type=secret,id=tok true\n"), 0o600))
	return dir
}

func TestForManifestDeclaredDockerfileIsTheBuild(t *testing.T) {
	dir := declaredProject(t)
	// An Airflow 2 pin, which RuntimeImage refuses: resolving a base for a
	// declared Dockerfile would fail this, so passing proves none is resolved.
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     dir,
		AirflowVersion: "2.9",
		Dockerfile:     "docker/Dockerfile",
	})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "docker", "Dockerfile"), req.Dockerfile)
	assert.Equal(t, dir, req.Context, "a declared Dockerfile builds with the project as its context")
	assert.Empty(t, req.BaseImage, "a declared Dockerfile names its own FROM")
	assert.True(t, req.FromDeclaredDockerfile())
}

func TestForManifestGeneratesFromTheAirflowPin(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
	})
	require.NoError(t, err)
	assert.Equal(t, RuntimeImageRepo+":3.1", req.BaseImage)
	assert.Empty(t, req.Dockerfile)
	assert.Empty(t, req.Context, "a generated build writes its own context under WorkDir")
	assert.False(t, req.FromDeclaredDockerfile())
}

// A [tool.astro] runtime names the build the generated image starts FROM, in
// place of the series tag.
func TestForManifestBuildsFromTheRuntimeBuild(t *testing.T) {
	for _, pin := range []string{"3.3", "3.3.1", "3"} {
		req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: pin, Runtime: "3.3-8"})
		require.NoError(t, err, pin)
		assert.Equal(t, RuntimeImageRepo+":3.3-8", req.BaseImage, pin)
	}
	// Deploy and package stay Airflow 3 only, runtime or not.
	_, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "2.11", Runtime: "13.11.0"})
	assert.ErrorContains(t, err, "only Airflow 3")
}

func TestForManifestRefusesAPinWithNoRuntimeImage(t *testing.T) {
	for _, pin := range []string{"", "2.9"} {
		_, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: pin})
		assert.Error(t, err, "pin %q", pin)
	}
}

func TestForManifestCarriesDependenciesAndPackages(t *testing.T) {
	deps := []string{"apache-airflow==3.1.0", "pandas>=2"}
	pkgs := []string{"libpq-dev", "gcc"}
	for name, m := range map[string]ManifestBuild{
		"generated": {ProjectDir: t.TempDir(), AirflowVersion: "3.1"},
		"declared":  {ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"},
	} {
		m.Dependencies, m.Packages = deps, pkgs
		req, err := ForManifest(m)
		require.NoError(t, err, name)
		assert.Equal(t, deps, req.Dependencies, name)
		assert.Equal(t, pkgs, req.Packages, name)
	}
}

// The request ForManifest returns, handed to Build, installs the manifest's
// dependencies and packages over the pin's base.
func TestForManifestGeneratedRequestInstallsDependenciesAndPackages(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
		Dependencies:   []string{"apache-airflow==3.1.0", "pandas>=2"},
		Packages:       []string{"libpq-dev"},
	})
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p", built)

	reqs, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, requirementsName))
	require.NoError(t, err)
	assert.Equal(t, "pandas>=2\n", string(reqs))
	pkgs, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, packagesName))
	require.NoError(t, err)
	assert.Equal(t, "libpq-dev\n", string(pkgs))
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, "FROM "+RuntimeImageRepo+":3.1\n", string(df))
}

// FromDeclaredDockerfile is the build-secret rule: a secret reaches the build
// exactly when it reports true, which is exactly when a Dockerfile is declared.
// Both sides are asserted against the fixed expectation rather than against
// each other, since Build's own gate reads FromDeclaredDockerfile and the two
// would move together if it were wrong.
func TestFromDeclaredDockerfileMatchesWhereSecretsReachTheBuild(t *testing.T) {
	cases := []struct {
		name     string
		m        ManifestBuild
		declared bool
	}{
		{"generated", ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1", Dependencies: []string{"pandas"}}, false},
		{"declared", ManifestBuild{ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"}, true},
	}
	for _, tc := range cases {
		req, err := ForManifest(tc.m)
		require.NoError(t, err, tc.name)
		req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"
		req.Secrets = []string{"id=tok,env=TOK"}

		cmd := &fakeCmd{}
		_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
		require.NoError(t, err, tc.name)
		assert.Equal(t, tc.declared, req.FromDeclaredDockerfile(), tc.name)
		assert.Equal(t, tc.declared, hasCall(cmd.calls, "--secret id=tok,env=TOK"), "%s: %v", tc.name, cmd.calls)
	}
}

// With nothing to install, BuildLocal still builds the one-line `FROM <base>`
// at the requested platform, so the tag it returns names a single-platform
// image in the local store rather than the base's multi-platform index.
func TestBuildLocalBuildsEvenWithNothingToInstall(t *testing.T) {
	req, err := ForManifest(ManifestBuild{
		ProjectDir:     t.TempDir(),
		AirflowVersion: "3.1",
		Dependencies:   []string{"apache-airflow==3.1.*"},
	})
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin, req.Platform = t.TempDir(), "astro-deploy/p", "docker", "linux/amd64"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-deploy/p", built, "the built tag, never the base")
	require.Len(t, cmd.calls, 1, "one build and nothing else, got %v", cmd.calls)
	assert.Contains(t, cmd.calls[0], "docker build --tag astro-deploy/p")
	assert.Contains(t, cmd.calls[0], "--platform linux/amd64")
	assert.False(t, hasCall(cmd.calls, "docker pull"), "%v", cmd.calls)

	// The ONBUILD triggers COPY both files, so both exist, and install nothing.
	reqs, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, requirementsName))
	require.NoError(t, err)
	assert.Equal(t, "\n", string(reqs))
	pkgs, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, packagesName))
	require.NoError(t, err)
	assert.Empty(t, pkgs)
	df, err := os.ReadFile(filepath.Join(req.WorkDir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, "FROM "+RuntimeImageRepo+":3.1\n", string(df))
}

// Build keeps its fast path: local docker mode runs the base as-is at the host
// platform when there is nothing to install.
func TestBuildKeepsTheFastPathBuildLocalSkips(t *testing.T) {
	req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1"})
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-local/p", "docker"

	cmd := &fakeCmd{}
	built, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.BaseImage, built)
	assert.Empty(t, cmd.calls)
}

func TestBuildLocalReturnsTheTagForEveryMode(t *testing.T) {
	for name, m := range map[string]ManifestBuild{
		"generated with deps": {ProjectDir: t.TempDir(), AirflowVersion: "3.1", Dependencies: []string{"pandas"}},
		"declared":            {ProjectDir: declaredProject(t), Dockerfile: "docker/Dockerfile"},
	} {
		req, err := ForManifest(m)
		require.NoError(t, err, name)
		req.WorkDir, req.Tag, req.Bin, req.Platform = t.TempDir(), "astro-deploy/p", "docker", "linux/amd64"

		cmd := &fakeCmd{}
		built, err := testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
		require.NoError(t, err, name)
		assert.Equal(t, "astro-deploy/p", built, name)
		assert.False(t, hasCall(cmd.calls, "docker pull"), "%s: %v", name, cmd.calls)
	}
}

func TestBuildLocalNamesAFailedBuild(t *testing.T) {
	req, err := ForManifest(ManifestBuild{ProjectDir: t.TempDir(), AirflowVersion: "3.1"})
	require.NoError(t, err)
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-deploy/p", "docker"

	boom := errors.New("registry unreachable")
	cmd := &fakeCmd{run: func(string, rt.Stdio) error { return boom }}
	_, err = testBuilder(cmd).BuildLocal(context.Background(), req, rt.Callbacks{})
	require.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "see the build output above")
}
