package imagebuild

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A generated build's survey lists what copyProjectFiles copies, and counts
// the DAGs on disk apart from those the image carries.
func TestSurveyProjectGenerated(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"dags/a.py":           "",
		"dags/sub/b.py":       "",
		"dags/README.md":      "",
		"include/x.py":        "",
		".env":                "SECRET=x\n",
		".git/config":         "",
		"requirements.txt":    "",
		"certs/prod.pem":      "",
		".dockerignore":       "dags/sub/\n",
		"include/.env.sample": "",
	})
	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dags/a.py", "dags/README.md", "include/x.py", "certs/prod.pem"}, s.Files)
	assert.Equal(t, 2, s.DagsOnDisk)
	assert.Equal(t, 1, s.DagsCarried)

	dst := t.TempDir()
	require.NoError(t, copyProjectFiles(project, dst))
	assert.ElementsMatch(t, s.Files, contextFiles(t, dst), "the survey and the copy agree")
}

// A .dockerignore that drops every DAG leaves none carried, which a deploy
// that bakes DAGs into the image refuses.
func TestSurveyProjectDagsDropped(t *testing.T) {
	for name, rules := range map[string]string{
		"the directory":   "dags/\n",
		"its contents":    "dags/**\n",
		"every py":        "**/*.py\n",
		"everything":      "*\n",
		"none (no dags/)": "",
	} {
		project := t.TempDir()
		files := map[string]string{".dockerignore": rules}
		if rules != "" {
			files["dags/a.py"] = ""
		}
		writeTree(t, project, files)
		s, err := SurveyProject(project, "", false)
		require.NoError(t, err, name)
		assert.Equal(t, 0, s.DagsCarried, name)
		if rules != "" {
			assert.Equal(t, 1, s.DagsOnDisk, name)
		} else {
			assert.Equal(t, 0, s.DagsOnDisk, name)
		}
	}
}

// A declared Dockerfile's context is the project under its ignore file alone:
// no built-in rule, version-control metadata not listed, and the Dockerfile's
// own ignore file read in place of the context's.
func TestSurveyProjectDeclared(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"docker/Dockerfile": "FROM x\n",
		"dags/a.py":         "",
		".env":              "SECRET=x\n",
		".git/config":       "",
		"requirements.txt":  "",
		".dockerignore":     "dags/\n",
		"tmp/scratch":       "",
	})
	s, err := SurveyProject(project, "docker/Dockerfile", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"docker/Dockerfile", ".env", "requirements.txt", ".dockerignore", "tmp/scratch"}, s.Files)
	assert.Equal(t, 1, s.DagsOnDisk)
	assert.Equal(t, 0, s.DagsCarried)

	writeTree(t, project, map[string]string{"docker/Dockerfile.dockerignore": "tmp/\n"})
	s, err = SurveyProject(project, "docker/Dockerfile", false)
	require.NoError(t, err)
	assert.Contains(t, s.Files, "dags/a.py", "the Dockerfile's own ignore file replaces .dockerignore")
	assert.NotContains(t, s.Files, "tmp/scratch")
	assert.Equal(t, 1, s.DagsCarried)
}

// A declared Dockerfile's context carries .git unless its ignore file drops
// it: each one is named, a submodule's .git file too, and none is entered.
func TestSurveyProjectDeclaredNamesVCSMetadata(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"Dockerfile":        "FROM x\n",
		".git/config":       "[core]\n",
		"vendor/lib/.git":   "gitdir: ../../.git/modules/lib\n",
		"vendor/lib/lib.py": "",
	})
	s, err := SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{".git", "vendor/lib/.git"}, s.VCS)
	assert.ElementsMatch(t, []string{"Dockerfile", "vendor/lib/lib.py"}, s.Files)

	writeTree(t, project, map[string]string{".dockerignore": "**/.git\n"})
	s, err = SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.Empty(t, s.VCS)
	assert.Equal(t, ".dockerignore", s.IgnoreFile)

	generated, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.Empty(t, generated.VCS, "a generated build never carries VCS metadata")
}

// Podman and buildah read a .containerignore before any .dockerignore;
// docker never reads one.
func TestSurveyProjectDeclaredReadsContainerignoreUnderPodmanOnly(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"Dockerfile":              "FROM x\n",
		"Dockerfile.dockerignore": "a/\n",
		".containerignore":        "b/\n",
		"a/x":                     "",
		"b/y":                     "",
	})
	s, err := SurveyProject(project, "Dockerfile", true)
	require.NoError(t, err)
	assert.Equal(t, ".containerignore", s.IgnoreFile)
	assert.Contains(t, s.Files, "a/x")
	assert.NotContains(t, s.Files, "b/y")

	s, err = SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.Equal(t, "Dockerfile.dockerignore", s.IgnoreFile, "docker never reads a .containerignore")
	assert.NotContains(t, s.Files, "a/x")
	assert.Contains(t, s.Files, "b/y")
}

// A declared build's context carries a link as a link, so it is listed and
// not entered; a generated build carries what it points at.
func TestSurveyProjectLinks(t *testing.T) {
	outside := t.TempDir()
	writeTree(t, outside, map[string]string{"dags/far.py": "", "keys/sa-prod.json": ""})
	project := t.TempDir()
	writeTree(t, project, map[string]string{"docker/Dockerfile": "FROM x\n"})
	if err := os.Symlink(filepath.Join(outside, "dags"), filepath.Join(project, "dags")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	require.NoError(t, os.Symlink(filepath.Join(outside, "keys"), filepath.Join(project, "keys")))

	generated, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"docker/Dockerfile", "dags/far.py", "keys/sa-prod.json"}, generated.Files)
	assert.Equal(t, 1, generated.DagsOnDisk)
	assert.Equal(t, 1, generated.DagsCarried)

	declared, err := SurveyProject(project, "docker/Dockerfile", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"docker/Dockerfile", "dags", "keys"}, declared.Files)
	assert.Equal(t, 1, declared.DagsOnDisk)
	assert.Equal(t, 0, declared.DagsCarried)
}
