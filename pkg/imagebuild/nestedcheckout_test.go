package imagebuild

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "protocol.file.allow=always"}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
}

// nestedProject is a project repository holding, below its root, a linked
// worktree of itself (.claude/worktrees/x, a .git file), an untracked clone
// (scratch/, a .git directory), a submodule (vendor/sub) and a repository its
// index records as a gitlink with no .gitmodules entry (vendor/embedded).
func nestedProject(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	project := t.TempDir()
	gitIn(t, project, "init", "-q", "-b", "main")
	writeTree(t, project, map[string]string{
		"dags/a.py":          "",
		"deploy/sandbox.env": "MODE=sandbox\n",
		".gitignore":         ".claude/worktrees/\nscratch/\n",
	})
	gitIn(t, project, "add", ".")
	gitIn(t, project, "commit", "-q", "-m", "project")
	gitIn(t, project, "worktree", "add", "-q", "-b", "agent", filepath.Join(project, ".claude", "worktrees", "x"))

	scratch := filepath.Join(project, "scratch")
	require.NoError(t, os.MkdirAll(scratch, 0o755))
	gitIn(t, scratch, "init", "-q")
	writeTree(t, scratch, map[string]string{"notes.txt": "n"})

	upstream := t.TempDir()
	gitIn(t, upstream, "init", "-q", "-b", "main")
	writeTree(t, upstream, map[string]string{"lib.py": ""})
	gitIn(t, upstream, "add", ".")
	gitIn(t, upstream, "commit", "-q", "-m", "lib")
	gitIn(t, project, "submodule", "add", "-q", upstream, "vendor/sub")

	embedded := filepath.Join(project, "vendor", "embedded")
	require.NoError(t, os.MkdirAll(embedded, 0o755))
	gitIn(t, embedded, "init", "-q", "-b", "main")
	writeTree(t, embedded, map[string]string{"util.py": ""})
	gitIn(t, embedded, "add", ".")
	gitIn(t, embedded, "commit", "-q", "-m", "util")
	gitIn(t, project, "add", "vendor/embedded")
	return project
}

// A generated build leaves out only a linked worktree of the project's own
// repository, silently; a stray clone, a submodule and a gitlinked
// repository are carried like any other directory. The survey, the copy and
// the digest agree.
func TestGeneratedBuildLeavesOutOnlyTheProjectsWorktrees(t *testing.T) {
	project := nestedProject(t)
	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"dags/a.py", "deploy/sandbox.env", ".gitignore", ".gitmodules",
		"scratch/notes.txt", "vendor/sub/lib.py", "vendor/embedded/util.py",
	}, s.Files)
	assert.Empty(t, s.Nested, "a generated build lists none: it carries none")

	dst := t.TempDir()
	require.NoError(t, copyProjectFiles(project, dst))
	assert.ElementsMatch(t, s.Files, contextFiles(t, dst))

	before, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	writeTree(t, project, map[string]string{".claude/worktrees/x/dags/new.py": ""})
	after, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	assert.Equal(t, before, after, "an edit inside the project's worktree does not move the image")
}

// A declared Dockerfile's context is the whole project, so it carries the
// project's worktrees; the survey names them, and only them, for the ship
// check to say where a file it flags sits.
func TestDeclaredSurveyNamesTheProjectsWorktrees(t *testing.T) {
	project := nestedProject(t)
	writeTree(t, project, map[string]string{"Dockerfile": "FROM python:3.12\n"})
	s, err := SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.Equal(t, []string{".claude/worktrees/x"}, s.Nested)
	assert.Contains(t, s.Files, ".claude/worktrees/x/deploy/sandbox.env")
	assert.Contains(t, s.Files, "scratch/notes.txt")

	writeTree(t, project, map[string]string{".dockerignore": ".claude/\n"})
	s, err = SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.Empty(t, s.Nested, "a worktree its ignore file leaves out is not named")
}

// The rule reads files only. A .git file whose gitdir, relative or absolute,
// points under the project's .git/worktrees/ is a worktree of the project and
// is left out; one pointing anywhere else (another repository's worktree, a
// submodule's .git/modules/) is carried.
func TestLinkedWorktreeIsReadFromFiles(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git", "worktrees", "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(project, ".git", "modules", "lib"), 0o755))
	elsewhere := t.TempDir()
	writeTree(t, project, map[string]string{
		"dags/d.py":         "",
		"wt/a/.git":         "gitdir: ../../.git/worktrees/a\n",
		"wt/a/dags/copy.py": "",
		"wt/other/.git":     "gitdir: " + filepath.Join(elsewhere, ".git", "worktrees", "o") + "\n",
		"wt/other/keep.py":  "",
		"vendor/lib/.git":   "gitdir: ../../.git/modules/lib\n",
		"vendor/lib/lib.py": "",
	})
	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dags/d.py", "wt/other/keep.py", "vendor/lib/lib.py"}, s.Files)
}

// A project that is not in a repository has no worktrees to leave out: a
// clone checked out under dags/ is the project's code, and is carried.
func TestNonGitProjectCarriesAClone(t *testing.T) {
	project := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(project, "dags", "shared", ".git"), 0o755))
	writeTree(t, project, map[string]string{
		"dags/shared/common.py": "",
		"dags/d.py":             "",
	})
	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dags/d.py", "dags/shared/common.py"}, s.Files)
}
