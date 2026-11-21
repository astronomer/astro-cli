package shipcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/fileutil"
)

// uploadOf is a Dags upload of the project at dir, as Astro Private Cloud's
// tars it: dags/, every entry under dags/.
func uploadOf(dir string) fileutil.TarSource {
	return fileutil.TarSource{Dir: filepath.Join(dir, "dags"), BaseDir: "dags"}
}

// checkNoDags checks a project with no dags/.
func checkNoDags(t *testing.T) error {
	dir := t.TempDir()
	return CheckDags(dir, uploadOf(dir), nil)
}

// A Dags-only upload is held to an image's credential check: a file under
// dags/ that looks like credentials and that git does not track is refused,
// with the way out a Dags upload has, and a committed one ships.
func TestCheckDagsRefusesAnUntrackedCredential(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		".gitignore":        "*.env\n*.pem\n",
		"dags/etl.py":       "import x\n",
		"dags/settings.env": "TOKEN=x\n",
		"dags/ca.pem":       "committed",
	})
	gitRun(t, dir, "add", ".gitignore", "dags/etl.py")
	gitRun(t, dir, "add", "-f", "dags/ca.pem")

	err := CheckDags(dir, uploadOf(dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to upload the Dags")
	assert.Contains(t, err.Error(), "dags/settings.env")
	assert.Contains(t, err.Error(), "Move it out of dags/, or commit the untracked one if it is meant to ship")
	assert.NotContains(t, err.Error(), "ca.pem", "a committed file ships, as in an image")
	assert.NotContains(t, err.Error(), ".dockerignore", "a Dags upload reads no ignore file")
}

// What the upload does not carry is not held against it: a link inside
// dags/ goes up as a link, not the file it points at, and bytecode caches stay
// behind. A project with nothing to object to, or no dags/, uploads.
func TestCheckDagsLooksOnlyAtWhatTheUploadCarries(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		".gitignore":               ".env\n__pycache__/\n",
		".env":                     "TOKEN=x\n",
		"dags/etl.py":              "import x\n",
		"dags/__pycache__/key.pem": "x",
	})
	gitRun(t, dir, "add", ".gitignore", "dags/etl.py")
	if err := os.Symlink(filepath.Join("..", ".env"), filepath.Join(dir, "dags", "local.env")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	assert.NoError(t, CheckDags(dir, uploadOf(dir), nil))

	assert.NoError(t, checkNoDags(t), "no dags/ is nothing to upload")
}

// A dags/ that is a link out of the project uploads what it leads to, so a
// credential there is refused as one reached through a link out of the
// project is in an image.
func TestCheckDagsRefusesACredentialBehindALinkedDagsDirectory(t *testing.T) {
	needsGit(t)
	dir, shared := t.TempDir(), t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, shared, map[string]string{
		"etl.py":          "import x\n",
		"service-key.pem": "k",
	})
	if err := os.Symlink(shared, filepath.Join(dir, "dags")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	err := CheckDags(dir, uploadOf(dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dags/service-key.pem")
	assert.Contains(t, err.Error(), "reached through a link out of the project")
	assert.NotContains(t, err.Error(), "commit", "a file behind a link out of the project is refused committed or not")
}

// A dags/ linked to a directory that is itself named __pycache__ is walked,
// as the uploads walk it: only a cache below the root is left out.
func TestCheckDagsWalksADagsDirectoryNamedLikeACache(t *testing.T) {
	dir, shared := t.TempDir(), t.TempDir()
	cache := filepath.Join(shared, "__pycache__")
	writeFiles(t, cache, map[string]string{"service-key.pem": "k"})
	if err := os.Symlink(cache, filepath.Join(dir, "dags")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}

	err := CheckDags(dir, uploadOf(dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dags/service-key.pem")
}

// A dags/ that cannot be resolved for a reason other than not being there (a
// link to itself) fails the check rather than passing it unread.
func TestCheckDagsFailsOnADagsDirectoryItCannotResolve(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("dags", filepath.Join(dir, "dags")); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	err := CheckDags(dir, uploadOf(dir), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading the project's Dags")
}

// Outside a git work tree there is nothing to ask, as for an image: a file
// that looks like credentials under dags/ uploads, and nothing is warned.
func TestCheckDagsOutsideGitAsksNothing(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"dags/etl.py": "import x\n", "dags/settings.env": "TOKEN=x\n"})
	var warned []string
	require.NoError(t, CheckDags(dir, uploadOf(dir), func(w string) { warned = append(warned, w) }))
	assert.Empty(t, warned)
}

// What is checked is what the upload's own TarSource carries: an entry its
// excludes leave out is not held against it, and the same entry is when the
// upload carries it (the excludes match the name in the tarball, so one
// written without the dags/ prefix matches only a bundle without it).
func TestCheckDagsChecksWhatItsSourceCarries(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		".gitignore":            "*.pem\n",
		"dags/etl.py":           "import x\n",
		"dags/certs/deploy.pem": "k",
	})
	gitRun(t, dir, "add", ".gitignore", "dags/etl.py")

	dags := filepath.Join(dir, "dags")
	excluded := fileutil.GzSource(dags, false, []string{"certs/"})
	assert.NoError(t, CheckDags(dir, excluded, nil), "an entry the upload leaves out")

	carried := fileutil.GzSource(dags, true, []string{"certs/"})
	err := CheckDags(dir, carried, nil)
	require.Error(t, err, "the same entry, carried")
	assert.Contains(t, err.Error(), "dags/certs/deploy.pem")
}

// Git metadata under dags/, at any depth, is not uploaded, so it is not held
// against the upload either: a nested checkout's .git/config, remote URL and
// all, stays behind, on Astro and Astro Private Cloud alike.
func TestCheckDagsLeavesGitMetadataBehind(t *testing.T) {
	needsGit(t)
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	writeFiles(t, dir, map[string]string{
		".gitignore":                 "*.pem\n",
		"dags/etl.py":                "import x\n",
		"dags/.git/deploy.pem":       "k",
		"dags/vendored/.git/key.pem": "k",
	})
	gitRun(t, dir, "add", ".gitignore", "dags/etl.py")
	for _, prefixed := range []bool{true, false} {
		assert.NoError(t, CheckDags(dir, fileutil.GzSource(filepath.Join(dir, "dags"), prefixed, nil), nil))
	}
	assert.NoError(t, CheckDags(dir, uploadOf(dir), nil))
}
