package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// baseProject is a project whose Dockerfile astro init moved onto the
// install base: FROM ${ASTRO_BASE}, then the project's files and its own step.
func baseProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	df := airflowrt.BaseArgCheckDirective + "\nARG ASTRO_BASE\nFROM ${ASTRO_BASE}\nCOPY --chown=astro:0 . .\nRUN echo own\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(df), 0o600))
	return dir
}

// baseRequest is a build of baseProject's Dockerfile over runtime 3.3-8.
func baseRequest(t *testing.T, project string) Request {
	return Request{
		WorkDir:    t.TempDir(),
		BaseImage:  "astrocrpublic.azurecr.io/runtime:3.3-8",
		Dockerfile: filepath.Join(project, "Dockerfile"),
		Context:    project,
		Tag:        "img:1",
		Bin:        "docker",
		Install:    &InstallLists{Requirements: []string{"pandas==2.2.3"}, Constraints: []string{"numpy==2.1.0"}, Packages: []string{"jq"}},
	}
}

// A Dockerfile built on the install base is two builds: the base, over the
// runtime's -base tag with the lists in a directory of the work dir, then the
// project's own file with the base's tag as ASTRO_BASE. Nothing is written
// into the project.
func TestBuildBuildsTheInstallBaseFirst(t *testing.T) {
	project := baseProject(t)
	cmd := &fakeCmd{}
	req := baseRequest(t, project)
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	base := req.InstallBaseTag()
	assert.True(t, strings.HasPrefix(base, "localhost/"+tagName(project)+"-base:"), base)
	dir := filepath.Join(req.WorkDir, installBaseDir)
	require.Len(t, cmd.calls, 2)
	assert.Contains(t, cmd.calls[0], "build --tag "+base+" --file "+filepath.Join(dir, dockerfileName)+" --pull "+filepath.Join(dir, buildContextDir))
	assert.Contains(t, cmd.calls[1], " --build-arg ASTRO_BASE="+base+" "+project)
	assert.NotContains(t, cmd.calls[1], "--pull", "the base is local")
	df, err := os.ReadFile(filepath.Join(dir, dockerfileName))
	require.NoError(t, err)
	assert.Equal(t, airflowrt.InstallBaseDockerfile("astrocrpublic.azurecr.io/runtime:3.3-8-base", false), string(df))
	for name, want := range req.Install.Files() {
		got, err := os.ReadFile(filepath.Join(dir, buildContextDir, name))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), name)
	}
	entries, err := os.ReadDir(project)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the Dockerfile: nothing written into the project")
}

// The base mounts the netrc secret only when the build has it, and gets no
// other secret; the project's build gets every one.
func TestInstallBaseTakesOnlyTheNetrcSecret(t *testing.T) {
	cmd := &fakeCmd{}
	req := baseRequest(t, baseProject(t))
	req.Secrets = []string{"id=netrc,src=/n", "id=other,env=X"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Contains(t, cmd.calls[0], "--secret id=netrc,src=/n")
	assert.NotContains(t, cmd.calls[0], "id=other")
	assert.Contains(t, cmd.calls[1], "--secret id=netrc,src=/n --secret id=other,env=X")
	df, err := os.ReadFile(filepath.Join(req.WorkDir, installBaseDir, dockerfileName))
	require.NoError(t, err)
	assert.Contains(t, string(df), "--mount=type=secret,id=netrc")
}

// The base's tag moves with what it installs, and only with that.
func TestInstallBaseTagFollowsTheInstall(t *testing.T) {
	req := baseRequest(t, baseProject(t))
	tag := req.InstallBaseTag()
	assert.Equal(t, tag, req.InstallBaseTag())
	other := req
	other.Install = &InstallLists{Requirements: []string{"pandas==2.2.4"}}
	assert.NotEqual(t, tag, other.InstallBaseTag())
	other = req
	other.BaseImage = "astrocrpublic.azurecr.io/runtime:3.3-9"
	assert.NotEqual(t, tag, other.InstallBaseTag())
	other = req
	other.Tag = "img:2"
	assert.Equal(t, tag, other.InstallBaseTag(), "the final image's tag is not the base's business")
}

// InstallBaseOnly builds the base and nothing else.
func TestBuildInstallBaseOnly(t *testing.T) {
	cmd := &fakeCmd{}
	req := baseRequest(t, baseProject(t))
	req.InstallBaseOnly = true
	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, req.InstallBaseTag(), got)
	assert.Len(t, cmd.calls, 1)

	_, err = testBuilder(&fakeCmd{}).Build(context.Background(), Request{Dockerfile: filepath.Join(declaredProject(t), "docker", "Dockerfile"), Context: t.TempDir(), InstallBaseOnly: true}, rt.Callbacks{})
	require.ErrorContains(t, err, "does not build FROM ${ASTRO_BASE}")
}

// Without a runtime to build the base on, the build says so.
func TestInstallBaseNeedsARuntime(t *testing.T) {
	req := baseRequest(t, baseProject(t))
	req.BaseImage = ""
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.ErrorContains(t, err, "no runtime was resolved")
}

// Without resolved lists it installs the request's own fields, unpinned, as
// a generated build does.
func TestInstallBaseInstallsUnpinnedListsWithoutInstall(t *testing.T) {
	req := baseRequest(t, baseProject(t))
	req.Install = nil
	req.Dependencies = []string{"apache-airflow==2.10.*", "pandas"}
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(req.WorkDir, installBaseDir, buildContextDir, "requirements.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pandas\n", string(got))
}

// Any other Dockerfile is built as before, alone, with no build argument.
func TestBuildBuildsNoInstallBaseForOtherDockerfiles(t *testing.T) {
	dir := declaredProject(t)
	cmd := &fakeCmd{}
	req := Request{WorkDir: t.TempDir(), Dockerfile: filepath.Join(dir, "docker", "Dockerfile"), Context: dir, Tag: "img:1", Bin: "docker"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	require.Len(t, cmd.calls, 1)
	assert.NotContains(t, cmd.calls[0], "--build-arg")
	assert.NoDirExists(t, filepath.Join(req.WorkDir, installBaseDir))
}

// The written layout: the project under its ignore file, its Dockerfile
// included, and the base's build beside it.
func TestWriteDeclaredContextLayout(t *testing.T) {
	project := baseProject(t)
	for name, body := range map[string]string{
		".dockerignore":    ".env\nsecret/\nDockerfile\n",
		".env":             "A=1\n",
		"secret/key":       "k\n",
		"dags/dag.py":      "x = 1\n",
		"requirements.txt": "old\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Join(project, filepath.Dir(name)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(project, name), []byte(body), 0o600))
	}
	if runtime.GOOS != "windows" {
		require.NoError(t, os.Symlink("dags/dag.py", filepath.Join(project, "link.py")))
	}
	out := filepath.Join(t.TempDir(), "build")
	req := baseRequest(t, project)

	base, err := WriteDeclaredContext(out, req)
	require.NoError(t, err)
	assert.True(t, base)
	projectDir := filepath.Join(out, DeclaredProjectDir)
	df, err := os.ReadFile(filepath.Join(projectDir, "Dockerfile"))
	require.NoError(t, err)
	assert.True(t, airflowrt.UsesInstallBase(df), "the Dockerfile is carried whatever the ignore file says")
	assert.FileExists(t, filepath.Join(projectDir, "dags", "dag.py"))
	assert.FileExists(t, filepath.Join(projectDir, "requirements.txt"), "the context is the project as docker reads it")
	assert.NoFileExists(t, filepath.Join(projectDir, ".env"))
	assert.NoDirExists(t, filepath.Join(projectDir, "secret"))
	if runtime.GOOS != "windows" {
		target, err := os.Readlink(filepath.Join(projectDir, "link.py"))
		require.NoError(t, err)
		assert.Equal(t, "dags/dag.py", target, "a link is carried as itself")
	}
	for name, want := range req.Install.Files() {
		got, err := os.ReadFile(filepath.Join(out, DeclaredBaseDir, name))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), name)
	}
	baseDF, err := os.ReadFile(filepath.Join(out, DeclaredBaseDir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, airflowrt.InstallBaseDockerfile("astrocrpublic.azurecr.io/runtime:3.3-8-base", false), string(baseDF),
		"no netrc mount without the secret, so the legacy builder runs it")

	_, err = WriteDeclaredContext(out, req)
	require.ErrorIs(t, err, ErrContextDirNotEmpty)
}

// A context written inside the project (`--context-dir ./build`) does not
// carry a copy of itself.
func TestWriteDeclaredContextInsideTheProject(t *testing.T) {
	project := baseProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(project, "dags"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(project, "dags", "dag.py"), []byte("x = 1\n"), 0o600))
	out := filepath.Join(project, "build")

	_, err := WriteDeclaredContext(out, baseRequest(t, project))
	require.NoError(t, err)
	projectDir := filepath.Join(out, DeclaredProjectDir)
	assert.FileExists(t, filepath.Join(projectDir, "dags", "dag.py"))
	assert.NoDirExists(t, filepath.Join(projectDir, "build"), "the output directory is not in its own context")
}

// A Dockerfile that names its own FROM gets no base.
func TestWriteDeclaredContextWithoutTheInstallBase(t *testing.T) {
	dir := declaredProject(t)
	out := filepath.Join(t.TempDir(), "build")
	base, err := WriteDeclaredContext(out, Request{Dockerfile: filepath.Join(dir, "docker", "Dockerfile"), Context: dir})
	require.NoError(t, err)
	assert.False(t, base)
	assert.NoDirExists(t, filepath.Join(out, DeclaredBaseDir))
	assert.FileExists(t, filepath.Join(out, DeclaredProjectDir, "docker", "Dockerfile"))
}

func TestTagName(t *testing.T) {
	assert.Equal(t, "my-project", tagName("/x/My Project"))
	assert.Equal(t, "astro", tagName("/x/__"))
}

// A Dockerfile built on the install base gets the base a generated build
// would, pulled from the manifest's registry; one with its own FROM gets none.
func TestForManifestResolvesTheInstallBase(t *testing.T) {
	project := baseProject(t)
	calls := 0
	base := func(context.Context, ManifestBuild) (string, error) {
		calls++
		return "astrocrpublic.azurecr.io/runtime:3.3-8", nil
	}
	req, err := ForManifestWith(context.Background(), ManifestBuild{ProjectDir: project, Dockerfile: "Dockerfile", Registry: "registry.example/astronomer"}, base)
	require.NoError(t, err)
	assert.Equal(t, "registry.example/astronomer/runtime:3.3-8", req.BaseImage)
	assert.True(t, req.UsesInstallBase())
	assert.Empty(t, req.ProjectFiles, "the Dockerfile's context is the project already")

	own := declaredProject(t)
	req, err = ForManifestWith(context.Background(), ManifestBuild{ProjectDir: own, Dockerfile: "docker/Dockerfile"}, base)
	require.NoError(t, err)
	assert.Empty(t, req.BaseImage)
	assert.Equal(t, 1, calls, "no base is resolved for a Dockerfile that names its own FROM")
}
