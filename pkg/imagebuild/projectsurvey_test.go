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

// A generated image leaves out the Cosmos Boost files an earlier Astro CLI
// wrote into the project, at any depth and dags/ included, judged by each
// file's own marker: the plugin would read them as current. One from another
// producer is the user's and is carried. The survey, the copy and the digest
// agree.
func TestGeneratedImageLeavesOutStaleCosmosBoostFiles(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"dags/dbt/shop/dbt_project.yml":                  "name: shop\n",
		"dags/dbt/shop/.astro/dbt_metadata.json":         `{"generated_by":{"application":"astro"}}`,
		"dags/dbt/shop/target/.astro/manifest.slim.json": `{"_generated_by":{"application":"astro"}}`,
		"include/dbt/.astro/dbt_metadata.json":           `{"generated_by":{"application":"someone-else"}}`,
		"include/dbt/.astro/notes.json":                  `{"generated_by":{"application":"astro"}}`,
	})
	want := []string{"dags/dbt/shop/dbt_project.yml", "include/dbt/.astro/dbt_metadata.json", "include/dbt/.astro/notes.json"}

	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, want, s.Files)
	assert.Empty(t, s.CosmosBoost, "a generated build lists none: it carries none")

	dst := t.TempDir()
	require.NoError(t, copyProjectFiles(project, dst))
	assert.ElementsMatch(t, want, contextFiles(t, dst))

	before, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	writeTree(t, project, map[string]string{"dags/dbt/shop/.astro/dbt_metadata.json": `{"generated_by":{"application":"astro"},"version":{"hash":"changed"}}`})
	after, err := ProjectFilesDigest(project)
	require.NoError(t, err)
	assert.Equal(t, before, after, "a file the image leaves out does not move its tag")
}

// A declared Dockerfile's context carries them, and the survey names them for
// the warning.
func TestSurveyProjectDeclaredNamesStaleCosmosBoostFiles(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"Dockerfile":                         "FROM x\n",
		"dbt/.astro/dbt_metadata.json":       `{"generated_by":{"application":"astro"}}`,
		"dbt/other/.astro/dbt_metadata.json": `{"generated_by":{"application":"someone-else"}}`,
	})
	s, err := SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.Contains(t, s.Files, "dbt/.astro/dbt_metadata.json", "the declared context carries it")
	assert.Equal(t, []string{"dbt/.astro/dbt_metadata.json"}, s.CosmosBoost)
}

// A link or a directory at a sidecar's path is not a sidecar this CLI wrote,
// but the plugin would read whatever it leads to as one: a generated image
// leaves it out, and a declared context's survey names it.
func TestNonRegularEntriesAtSidecarPaths(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"Dockerfile":                              "FROM x\n",
		"dags/dbt/shop/dbt_project.yml":           "name: shop\n",
		"include/stale.json":                      `{"generated_by":{"application":"someone-else"}}`,
		"dags/dbt/dir/.astro/dbt_metadata.json/x": "x",
	})
	link := filepath.Join(project, "dags", "dbt", "shop", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
	if err := os.Symlink(filepath.Join(project, "include", "stale.json"), link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}

	s, err := SurveyProject(project, "", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Dockerfile", "dags/dbt/shop/dbt_project.yml", "include/stale.json"}, s.Files)
	assert.Empty(t, s.CosmosBoost)
	dst := t.TempDir()
	require.NoError(t, copyProjectFiles(project, dst))
	assert.ElementsMatch(t, s.Files, contextFiles(t, dst), "the survey and the copy agree")

	s, err = SurveyProject(project, "Dockerfile", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dags/dbt/shop/.astro/dbt_metadata.json", "dags/dbt/dir/.astro/dbt_metadata.json"}, s.CosmosBoost)
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
	assert.ElementsMatch(t, []string{"dags", "keys"}, generated.Links, "the links the walk went through")
	assert.Equal(t, 1, generated.DagsOnDisk)
	assert.Equal(t, 1, generated.DagsCarried)

	declared, err := SurveyProject(project, "docker/Dockerfile", false)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"docker/Dockerfile", "dags", "keys"}, declared.Files)
	assert.Equal(t, 1, declared.DagsOnDisk)
	assert.Equal(t, 0, declared.DagsCarried)
	assert.Empty(t, declared.Links, "a declared build carries links as links")
}
