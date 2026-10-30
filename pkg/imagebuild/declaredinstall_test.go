package imagebuild

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// rewrittenProject is a project whose Dockerfile astro init moved onto a
// -base tag: it reads the install lists from the named context.
func rewrittenProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	df := airflowrt.InstallDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base") + "RUN echo own\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte(df), 0o600))
	return dir
}

// A Dockerfile that reads the named context is built with it, the lists the
// request carries written into the work dir; nothing is written into the
// project.
func TestBuildPassesTheInstallContextToADockerfileThatReadsIt(t *testing.T) {
	project := rewrittenProject(t)
	cmd := &fakeCmd{}
	req := Request{
		WorkDir:    t.TempDir(),
		Dockerfile: filepath.Join(project, "Dockerfile"),
		Context:    project,
		Tag:        "img:1",
		Bin:        "docker",
		Install:    &InstallLists{Requirements: []string{"pandas==2.2.3"}, Constraints: []string{"numpy==2.1.0"}, Packages: []string{"jq"}},
	}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	dir := filepath.Join(req.WorkDir, airflowrt.InstallContext)
	require.Len(t, cmd.calls, 2)
	assert.Equal(t, "docker build --help", cmd.calls[0], "the builder is asked whether it takes a named context first")
	assert.Contains(t, cmd.calls[1], " --build-context "+airflowrt.InstallContext+"="+dir+" "+project)
	for name, want := range req.Install.Files() {
		got, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		assert.Equal(t, string(want), string(got), name)
	}
	entries, err := os.ReadDir(project)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only the Dockerfile: nothing written into the project")
}

// Without resolved lists it installs the request's own fields, unpinned, as
// a generated build does.
func TestBuildPassesUnpinnedListsWithoutInstall(t *testing.T) {
	project := rewrittenProject(t)
	req := Request{
		WorkDir: t.TempDir(), Dockerfile: filepath.Join(project, "Dockerfile"), Context: project,
		Tag: "img:1", Bin: "docker", Dependencies: []string{"apache-airflow==2.10.*", "pandas"},
	}
	_, err := testBuilder(&fakeCmd{}).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(req.WorkDir, airflowrt.InstallContext, "requirements.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pandas\n", string(got))
}

// Any other Dockerfile is built as before, with no named context.
func TestBuildPassesNoInstallContextToOtherDockerfiles(t *testing.T) {
	dir := declaredProject(t)
	cmd := &fakeCmd{}
	req := Request{WorkDir: t.TempDir(), Dockerfile: filepath.Join(dir, "docker", "Dockerfile"), Context: dir, Tag: "img:1", Bin: "docker"}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)
	assert.NotContains(t, cmd.calls[0], "--build-context")
	assert.NoDirExists(t, filepath.Join(req.WorkDir, airflowrt.InstallContext))
}

// The written layout: the Dockerfile beside the context, the context under
// the project's ignore file, and the lists for the named context.
func TestWriteDeclaredContextLayout(t *testing.T) {
	project := rewrittenProject(t)
	for name, body := range map[string]string{
		".dockerignore":    ".env\nsecret/\n",
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
	req := Request{Dockerfile: filepath.Join(project, "Dockerfile"), Context: project, Install: &InstallLists{Requirements: []string{"pandas==2.2.3"}}}

	lists, err := WriteDeclaredContext(out, req)
	require.NoError(t, err)
	assert.True(t, lists)
	df, err := os.ReadFile(filepath.Join(out, DeclaredDockerfileName))
	require.NoError(t, err)
	assert.True(t, airflowrt.ReadsInstallContext(df))
	ctxDir := filepath.Join(out, DeclaredContextDir)
	assert.FileExists(t, filepath.Join(ctxDir, "dags", "dag.py"))
	assert.FileExists(t, filepath.Join(ctxDir, "requirements.txt"), "the context is the project as docker reads it")
	assert.NoFileExists(t, filepath.Join(ctxDir, ".env"))
	assert.NoDirExists(t, filepath.Join(ctxDir, "secret"))
	if runtime.GOOS != "windows" {
		target, err := os.Readlink(filepath.Join(ctxDir, "link.py"))
		require.NoError(t, err)
		assert.Equal(t, "dags/dag.py", target, "a link is carried as itself")
	}
	reqs, err := os.ReadFile(filepath.Join(out, DeclaredInstallDir, "requirements.txt"))
	require.NoError(t, err)
	assert.Equal(t, "pandas==2.2.3\n", string(reqs))

	_, err = WriteDeclaredContext(out, req)
	require.ErrorIs(t, err, ErrContextDirNotEmpty)
}

// A Dockerfile that does not read the named context gets no lists.
func TestWriteDeclaredContextWithoutTheNamedContext(t *testing.T) {
	dir := declaredProject(t)
	out := filepath.Join(t.TempDir(), "build")
	lists, err := WriteDeclaredContext(out, Request{Dockerfile: filepath.Join(dir, "docker", "Dockerfile"), Context: dir})
	require.NoError(t, err)
	assert.False(t, lists)
	assert.NoDirExists(t, filepath.Join(out, DeclaredInstallDir))
	assert.FileExists(t, filepath.Join(out, DeclaredContextDir, "docker", "Dockerfile"))
}
