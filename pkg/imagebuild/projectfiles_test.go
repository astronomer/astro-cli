package imagebuild

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
)

// writeTree writes files (slash-separated path to content) under dir.
func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
}

// listTree lists the files and symlinks under dir, slash-separated and sorted.
func listTree(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	require.NoError(t, err)
	sort.Strings(out)
	return out
}

// sampleProject is a project with code in each shipped directory, the
// per-machine files that must never ship, and files outside the shipped
// directories.
func sampleProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"dags/a.py":                            "a",
		"dags/sub/b.py":                        "b",
		"dags/__pycache__/a.cpython-312.pyc":   "cache",
		"dags/stale.pyc":                       "cache",
		"plugins/x.py":                         "x",
		"plugins/fix_local_executor_pickle.py": "per-machine",
		"include/q.sql":                        "q",
		"include/.env":                         "SECRET=1",
		"include/.venv/bin/python":             "venv",
		"include/lib/.git/HEAD":                "ref",
		"tests/test_a.py":                      "t",
		"pyproject.toml":                       "[project]\n",
		".env":                                 "SECRET=1",
		".astro/standalone/airflow.db":         "db",
	})
	return dir
}

func TestProjectCodeShipsDagsOnlyWhenAsked(t *testing.T) {
	assert.Equal(t, []string{"plugins", "include"}, ProjectCode(false))
	assert.Equal(t, []string{"dags", "plugins", "include"}, ProjectCode(true))
}

func TestStageProjectFilesCopiesTheNamedDirectoriesWithoutPerMachineFiles(t *testing.T) {
	project := sampleProject(t)
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, ProjectCode(true)))

	assert.Equal(t, []string{"dags/a.py", "dags/sub/b.py", "include/q.sql", "plugins/x.py"}, listTree(t, ctx))
	got, err := os.ReadFile(filepath.Join(ctx, "dags", "sub", "b.py"))
	require.NoError(t, err)
	assert.Equal(t, "b", string(got))
}

func TestStageProjectFilesLeavesOutWhatItIsNotAskedFor(t *testing.T) {
	project := sampleProject(t)
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, ProjectCode(false)))

	assert.Equal(t, []string{"include/q.sql", "plugins/x.py"}, listTree(t, ctx))
}

func TestStageProjectFilesSkipsAMissingDirectory(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{"dags/a.py": "a"})
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, ProjectCode(true)))

	assert.Equal(t, []string{"dags/a.py"}, listTree(t, ctx))
}

func TestStageProjectFilesHonorsTheProjectsDockerignore(t *testing.T) {
	project := sampleProject(t)
	writeTree(t, project, map[string]string{
		".dockerignore":       "include/q.sql\ndags/sub\n# a comment\n",
		"include/keep.sql":    "k",
		"plugins/big/data.db": "d",
	})
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, ProjectCode(true)))

	assert.Equal(t, []string{"dags/a.py", "include/keep.sql", "plugins/big/data.db", "plugins/x.py"}, listTree(t, ctx))
}

// The per-machine rules come after the project's, so a "!" rule cannot bring
// one back, while it can still bring back what the project's own rules left
// out.
func TestStageProjectFilesPerMachineRulesWinOverAProjectException(t *testing.T) {
	project := sampleProject(t)
	writeTree(t, project, map[string]string{
		".dockerignore": "include\n!include/q.sql\n!include/.env\n",
	})
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, []string{"include"}))

	assert.Equal(t, []string{"include/q.sql"}, listTree(t, ctx))
}

func TestStageProjectFilesKeepsModesAndSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes and symlinks are not what Windows keeps")
	}
	project := t.TempDir()
	writeTree(t, project, map[string]string{"plugins/run.sh": "#!/bin/sh\n", "plugins/x.py": "x"})
	// Group-writable, which the usual umask would strip from a new file.
	require.NoError(t, os.Chmod(filepath.Join(project, "plugins", "run.sh"), 0o775))
	require.NoError(t, os.Symlink("x.py", filepath.Join(project, "plugins", "alias.py")))
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, []string{"plugins"}))

	info, err := os.Stat(filepath.Join(ctx, "plugins", "run.sh"))
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o775), info.Mode().Perm())
	target, err := os.Readlink(filepath.Join(ctx, "plugins", "alias.py"))
	require.NoError(t, err)
	assert.Equal(t, "x.py", target)
}

// A dags/ that is itself a link to a directory elsewhere ships what it points
// at, under dags/.
func TestStageProjectFilesFollowsALinkedDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a directory symlink needs a privilege Windows does not grant by default")
	}
	elsewhere := t.TempDir()
	writeTree(t, elsewhere, map[string]string{"shared.py": "s"})
	project := t.TempDir()
	require.NoError(t, os.Symlink(elsewhere, filepath.Join(project, "dags")))
	ctx := t.TempDir()

	require.NoError(t, stageProjectFiles(project, ctx, []string{"dags"}))

	assert.Equal(t, []string{"dags/shared.py"}, listTree(t, ctx))
}

func TestStageProjectFilesRefusesPathsOutsideTheProjectOrOverTheGeneratedFiles(t *testing.T) {
	project := t.TempDir()
	for _, p := range []string{"../elsewhere", "/abs", ".", "", "requirements.txt", "packages.txt"} {
		err := stageProjectFiles(project, t.TempDir(), []string{p})
		assert.Error(t, err, "%q", p)
	}
}

func TestBuildStagesProjectFilesIntoTheContext(t *testing.T) {
	project := sampleProject(t)
	var staged []string
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.HasPrefix(call, "docker build") {
			fields := strings.Fields(call)
			staged = listTree(t, fields[len(fields)-1])
		}
		return nil
	}}
	req := Request{
		WorkDir:      t.TempDir(),
		BaseImage:    "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:          "astro-deploy/p",
		ProjectDir:   project,
		ProjectFiles: ProjectCode(false),
		Bin:          "docker",
	}

	got, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	// Nothing to install, but files to copy: the fast path is not taken.
	assert.Equal(t, "astro-deploy/p", got)
	assert.Equal(t, []string{"include/q.sql", "packages.txt", "plugins/x.py", "requirements.txt"}, staged)
}

// Without ProjectFiles the context is the two dependency files, as local Docker
// mode and Astro Desktop build it, and nothing an earlier build staged under
// the same WorkDir survives into it.
func TestBuildWithoutProjectFilesStagesOnlyTheDependencyFiles(t *testing.T) {
	project := sampleProject(t)
	workDir := t.TempDir()
	var staged []string
	cmd := &fakeCmd{run: func(call string, _ rt.Stdio) error {
		if strings.HasPrefix(call, "docker build") {
			fields := strings.Fields(call)
			staged = listTree(t, fields[len(fields)-1])
		}
		return nil
	}}
	req := Request{
		WorkDir:      workDir,
		BaseImage:    "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:          "astro-local/p",
		ProjectDir:   project,
		ProjectFiles: ProjectCode(true),
		Dependencies: []string{"pandas"},
		Bin:          "docker",
	}
	_, err := testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	req.ProjectFiles = nil
	_, err = testBuilder(cmd).Build(context.Background(), req, rt.Callbacks{})
	require.NoError(t, err)

	assert.Equal(t, []string{"packages.txt", "requirements.txt"}, staged)
}

func TestBuildWithoutProjectFilesKeepsTheFastPath(t *testing.T) {
	cmd := &fakeCmd{}
	got, err := testBuilder(cmd).Build(context.Background(), Request{
		WorkDir:    t.TempDir(),
		BaseImage:  "astrocrpublic.azurecr.io/runtime:3.1",
		Tag:        "astro-local/p",
		ProjectDir: sampleProject(t),
		Bin:        "docker",
	}, rt.Callbacks{})
	require.NoError(t, err)
	assert.Equal(t, "astrocrpublic.azurecr.io/runtime:3.1", got)
	assert.Empty(t, cmd.calls)
}

// A declared Dockerfile builds the project as its context, so ProjectFiles has
// nothing to add and nothing is written.
func TestBuildDockerfileModeIgnoresProjectFiles(t *testing.T) {
	project := sampleProject(t)
	writeTree(t, project, map[string]string{"Dockerfile": "FROM astrocrpublic.azurecr.io/runtime:3.1\n"})
	workDir := t.TempDir()
	cmd := &fakeCmd{}
	_, err := testBuilder(cmd).Build(context.Background(), Request{
		WorkDir:      workDir,
		Tag:          "astro-local/p",
		Dockerfile:   filepath.Join(project, "Dockerfile"),
		Context:      project,
		ProjectDir:   project,
		ProjectFiles: ProjectCode(true),
		Bin:          "docker",
	}, rt.Callbacks{})
	require.NoError(t, err)
	assert.Empty(t, listTree(t, workDir))
	require.Len(t, cmd.calls, 1)
	assert.True(t, strings.HasSuffix(cmd.calls[0], " "+project), cmd.calls[0])
}

func TestProjectFilesDigestFollowsWhatShips(t *testing.T) {
	project := sampleProject(t)
	paths := ProjectCode(true)
	digest := func() string {
		t.Helper()
		d, err := ProjectFilesDigest(project, paths)
		require.NoError(t, err)
		return d
	}
	base := digest()
	assert.Equal(t, base, digest(), "the same files give the same digest")

	// What does not ship does not move it.
	writeTree(t, project, map[string]string{
		"dags/__pycache__/a.cpython-312.pyc": "other cache",
		"include/.env":                       "SECRET=2",
		"tests/test_a.py":                    "changed",
	})
	assert.Equal(t, base, digest())

	// An edit, a new file and a rename each do.
	writeTree(t, project, map[string]string{"plugins/x.py": "x2"})
	edited := digest()
	assert.NotEqual(t, base, edited)
	writeTree(t, project, map[string]string{"include/new.sql": ""})
	added := digest()
	assert.NotEqual(t, edited, added)
	require.NoError(t, os.Rename(filepath.Join(project, "include", "new.sql"), filepath.Join(project, "include", "renamed.sql")))
	assert.NotEqual(t, added, digest())
}

func TestForManifestSetsTheProjectDir(t *testing.T) {
	dir := t.TempDir()
	req, err := ForManifest(ManifestBuild{ProjectDir: dir, AirflowVersion: "3.1"}, nil)
	require.NoError(t, err)
	assert.Equal(t, dir, req.ProjectDir)
	assert.Empty(t, req.ProjectFiles, "shipping project files is the caller's choice")
}
