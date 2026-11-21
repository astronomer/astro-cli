package shipcheck

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	err := CheckDags(dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing to upload the Dags")
	assert.Contains(t, err.Error(), "dags/settings.env")
	assert.Contains(t, err.Error(), "Move it out of dags/")
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
	assert.NoError(t, CheckDags(dir, nil))

	assert.NoError(t, CheckDags(t.TempDir(), nil), "no dags/ is nothing to upload")
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

	err := CheckDags(dir, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dags/service-key.pem")
	assert.Contains(t, err.Error(), "reached through a link out of the project")
}
