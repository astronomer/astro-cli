package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

const baseDockerfile = "# Built on the install base.\nFROM localhost/astro-base\nCOPY --chown=astro:0 . .\nRUN echo own-step\n"

// baseProject is a project named "My Project" whose own Dockerfile is
// dockerfile, and the request ForManifest makes of it.
func baseProject(t *testing.T, dockerfile string) Request {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(dockerfile), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "d.py"), []byte("x"), 0o600))
	req, err := ForManifestWith(t.Context(), ManifestBuild{
		ProjectDir: dir, Name: "My Project", AirflowVersion: "3.3", Dockerfile: "Dockerfile", Registry: "registry.example.com/astronomer",
	}, func(_ context.Context, _ ManifestBuild) (string, error) { return RuntimeImageRepo + ":3.3-11", nil })
	require.NoError(t, err)
	req.Install = &InstallLists{Requirements: []string{"pandas==2.2.3"}, Packages: []string{"jq"}}
	req.WorkDir, req.Tag, req.Bin = t.TempDir(), "astro-local/p", "docker"
	return req
}

// A Dockerfile built FROM localhost/astro-base gets the base a generated
// build would, on the mirror the manifest names, and builds the install base
// first, then itself from a copy whose FROM names that exact base, with the
// project as its context and nothing written there.
func TestBuildBuildsTheInstallBaseFirst(t *testing.T) {
	req := baseProject(t, baseDockerfile)
	assert.Equal(t, "localhost/astro-base", req.InstallBase)
	assert.Equal(t, "registry.example.com/astronomer/runtime:3.3-11", req.BaseImage)
	assert.Empty(t, req.ProjectFiles, "the Dockerfile's own build takes the project as its context")

	cmd := &fakeCmd{}
	got, err := testBuilder(cmd).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astro-local/p", got)
	require.Len(t, cmd.calls, 2)
	base := req.InstallBaseTag()
	assert.True(t, strings.HasPrefix(base, "localhost/astro-base:"), base)
	assert.Contains(t, cmd.calls[0], "--tag "+base+" ")
	assert.NotContains(t, cmd.calls[0], "--tag localhost/astro-base ", "a CLI build never moves the plain tag")
	assert.Contains(t, cmd.calls[0], "--pull")
	assert.Contains(t, cmd.calls[0], "--label io.astronomer.install-base.project=My Project ")
	assert.Contains(t, cmd.calls[0], "--label io.astronomer.install-base.digest="+req.InstallDigest()+" ")
	assert.Contains(t, cmd.calls[0], "--label io.astronomer.install-base.from="+req.BaseImage+" ")
	pinned := filepath.Join(req.WorkDir, installBaseDir, "project", "Dockerfile")
	assert.Contains(t, cmd.calls[1], "--file "+pinned+" ")
	assert.True(t, strings.HasSuffix(cmd.calls[1], " "+req.Context))
	assert.NotContains(t, cmd.calls[1], "--pull", "the base is local; a pull cannot reach it")
	data, err := os.ReadFile(pinned)
	require.NoError(t, err)
	assert.Equal(t, strings.Replace(baseDockerfile, "FROM localhost/astro-base", "FROM "+base, 1), string(data))

	df, err := os.ReadFile(filepath.Join(req.WorkDir, installBaseDir, dockerfileName))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(df), "FROM registry.example.com/astronomer/runtime:3.3-11-base\n"))
	assert.NotContains(t, string(df), "COPY --chown=astro:0 . .", "the base carries no project files")
	assert.NotContains(t, string(df), "--mount", "no secret, so a plain RUN the legacy builder runs")
	lists, err := os.ReadFile(filepath.Join(req.WorkDir, installBaseDir, buildContextDir, requirementsName))
	require.NoError(t, err)
	assert.Equal(t, "pandas==2.2.3\n", string(lists))
	entries, err := os.ReadDir(req.Context)
	require.NoError(t, err)
	assert.Len(t, entries, 2, "nothing is written into the project")
}

// --base builds the install base alone, tagged both exactly and plainly, and
// returns the plain tag; a Dockerfile that names its own FROM has none, and
// builds as before.
func TestInstallBaseOnly(t *testing.T) {
	req := baseProject(t, baseDockerfile)
	req.InstallBaseOnly = true
	cmd := &fakeCmd{}
	got, err := testBuilder(cmd).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "localhost/astro-base", got)
	require.Len(t, cmd.calls, 1)
	assert.Contains(t, cmd.calls[0], "--tag "+req.InstallBaseTag()+" ")
	assert.Contains(t, cmd.calls[0], "--tag localhost/astro-base ")

	own := baseProject(t, "FROM astrocrpublic.azurecr.io/runtime:3.3-11\n")
	assert.Empty(t, own.BaseImage, "a Dockerfile naming its own FROM resolves no base")
	assert.Empty(t, own.InstallBase)
	cmd = &fakeCmd{}
	_, err = testBuilder(cmd).Build(t.Context(), own, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	own.InstallBaseOnly = true
	_, err = testBuilder(&fakeCmd{}).Build(t.Context(), own, rt.Callbacks{})
	assert.ErrorContains(t, err, "has no install base to build")
}

// The base mounts the netrc secret only when the build has it, and its tag
// moves with what it installs.
func TestInstallBaseSecretAndTag(t *testing.T) {
	req := baseProject(t, baseDockerfile)
	plain := req.InstallBaseTag()
	req.Secrets = []string{"id=netrc,src=/home/me/.netrc", "id=other,env=X"}
	assert.Contains(t, req.installBaseDockerfile(), "--mount=type=secret,id=netrc")
	cmd := &fakeCmd{}
	_, err := testBuilder(cmd).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.NotContains(t, cmd.calls[0], "id=other", "the base reads netrc alone")
	assert.Contains(t, cmd.calls[1], "--secret id=other,env=X", "the Dockerfile's own build gets every secret")
	req.Secrets = nil
	req.Install = &InstallLists{Requirements: []string{"pandas==2.2.4"}}
	assert.NotEqual(t, plain, req.InstallBaseTag())
}

// --context-dir's two plain contexts: the base's build, and the project as
// docker reads it, its Dockerfile naming the exact base, without the
// directory being written when it sits inside the project.
func TestWriteDeclaredContext(t *testing.T) {
	req := baseProject(t, baseDockerfile)
	require.NoError(t, os.WriteFile(filepath.Join(req.Context, ".dockerignore"), []byte("secret.txt\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(req.Context, "secret.txt"), []byte("x"), 0o600))
	out := filepath.Join(req.Context, "build")
	require.NoError(t, WriteDeclaredContext(out, req))

	base, err := os.ReadFile(filepath.Join(out, DeclaredBaseDir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, req.installBaseDockerfile(), string(base))
	assert.FileExists(t, filepath.Join(out, DeclaredBaseDir, requirementsName))
	project := filepath.Join(out, DeclaredProjectDir)
	df, err := os.ReadFile(filepath.Join(project, "Dockerfile"))
	require.NoError(t, err)
	assert.Contains(t, string(df), "FROM "+req.InstallBaseTag()+"\n")
	assert.FileExists(t, filepath.Join(project, "dags", "d.py"))
	assert.NoFileExists(t, filepath.Join(project, "secret.txt"), "the ignore file applies")
	assert.NoDirExists(t, filepath.Join(project, "build"), "the context never carries itself")

	assert.ErrorIs(t, WriteDeclaredContext(out, req), ErrContextDirNotEmpty)
	own := baseProject(t, "FROM astrocrpublic.azurecr.io/runtime:3.3-11\n")
	assert.ErrorContains(t, WriteDeclaredContext(t.TempDir(), own), "is its build context")
}

func TestWithRegistry(t *testing.T) {
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.3-8", WithRegistry("astrocrpublic.azurecr.io/runtime:3.3-8", ""))
	assert.Equal(t, "registry.example.com/astronomer/runtime:3.3-8", WithRegistry("astrocrpublic.azurecr.io/runtime:3.3-8", "registry.example.com/astronomer"))
	assert.Equal(t, "mirror:5000/astro-runtime:12.6.0", WithRegistry("quay.io/astronomer/astro-runtime:12.6.0", "mirror:5000"))
}
