package cosmosboost

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCleanupDefaultRootRemovesArtifact(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))
	t.Chdir(dir)

	require.NoError(t, cleanupErr())

	_, err := os.Stat(filepath.Join(dir, artifactRelPath))
	require.True(t, os.IsNotExist(err), "artifact under the default root must be removed")
}

func TestCleanupNothingToRemove(t *testing.T) {
	require.NoError(t, cleanupErr(t.TempDir()), "an already-clean tree is not an error")
}

func TestCleanupNonexistentRoot(t *testing.T) {
	require.Error(t, cleanupErr(filepath.Join(t.TempDir(), "does-not-exist")))
}

// TestCleanupTouchesOnlyTheRequestedPaths pins the command's scope: cleanup
// acts on the paths it was given and nothing else on the machine.
func TestCleanupTouchesOnlyTheRequestedPaths(t *testing.T) {
	requested := t.TempDir()
	writeDbtProject(t, requested)
	require.NoError(t, PreDeploy(requested))

	elsewhere := t.TempDir()
	writeDbtProject(t, elsewhere)
	require.NoError(t, PreDeploy(elsewhere))

	require.NoError(t, cleanupErr(requested))

	_, err := os.Stat(filepath.Join(requested, artifactRelPath))
	require.True(t, os.IsNotExist(err))
	require.FileExists(t, filepath.Join(elsewhere, artifactRelPath),
		"a path that was not requested must not be touched")
}

// cleanupErr is Cleanup for a test that reads only its error.
func cleanupErr(roots ...string) error {
	_, err := Cleanup(roots...)
	return err
}

// resolved is path with symlinks followed, as the cleanup walks it (on macOS
// a temp dir under /var is really under /private/var).
func resolved(t *testing.T, path string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	return r
}

// TestCleanupReportsWhatItDid: the report names each artifact removed and
// each one kept because another tool wrote it, which is what
// `astro dbt cleanup -o json` publishes.
func TestCleanupReportsWhatItDid(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))
	foreign := filepath.Join(dir, "other", ".astro", "dbt_metadata.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(foreign), 0o755))
	require.NoError(t, os.WriteFile(foreign, []byte(`{"generated_by": {"application": "someone-else"}}`), 0o600))

	report, err := Cleanup(dir)

	require.NoError(t, err)
	require.Equal(t, []string{dir}, report.Roots)
	require.Equal(t, []string{filepath.Join(resolved(t, dir), artifactRelPath)}, report.Removed)
	require.Equal(t, []string{resolved(t, foreign)}, report.Kept)
}

// TestCleanupReportsEachRootOnce: one directory given twice, as `.` and as
// its absolute path, is one root in the report.
func TestCleanupReportsEachRootOnce(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	wd, err := os.Getwd()
	require.NoError(t, err)

	report, err := Cleanup(".", wd)

	require.NoError(t, err)
	require.Equal(t, []string{wd}, report.Roots)
}

// symlinkTo makes link point at target, skipping the test where this
// platform or account cannot make one (Windows without the privilege).
func symlinkTo(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
}

// TestCleanupWalksASymlinkedRoot: a root that is a symlink to a project is
// cleaned at its target. The walk does not descend a root that is itself a
// link, so without resolving it first this removed nothing and succeeded.
// The report names the root as given.
func TestCleanupWalksASymlinkedRoot(t *testing.T) {
	target := t.TempDir()
	writeDbtProject(t, target)
	require.NoError(t, PreDeploy(target))
	link := filepath.Join(t.TempDir(), "proj")
	symlinkTo(t, target, link)

	report, err := Cleanup(link + string(filepath.Separator))

	require.NoError(t, err)
	require.Equal(t, []string{link}, report.Roots)
	require.Equal(t, []string{filepath.Join(resolved(t, target), artifactRelPath)}, report.Removed)
	require.NoFileExists(t, filepath.Join(target, artifactRelPath))
}

// TestCleanupWalksALinkAndItsTargetOnce: a symlink and its target given
// together are one directory, cleaned once, and named by the first spelling.
func TestCleanupWalksALinkAndItsTargetOnce(t *testing.T) {
	target := t.TempDir()
	writeDbtProject(t, target)
	require.NoError(t, PreDeploy(target))
	link := filepath.Join(t.TempDir(), "proj")
	symlinkTo(t, target, link)

	report, err := Cleanup(link, target)

	require.NoError(t, err)
	require.Equal(t, []string{link}, report.Roots)
	require.Len(t, report.Removed, 1)
	require.NoFileExists(t, filepath.Join(target, artifactRelPath))
}

// TestCleanupNamesAMissingRootAsGiven: the failure for a root that does not
// exist names it as it was typed.
func TestCleanupNamesAMissingRootAsGiven(t *testing.T) {
	t.Chdir(t.TempDir())

	_, err := Cleanup("missing-dir")

	require.ErrorContains(t, err, `scanning "missing-dir"`)
}

// TestCleanupFailsWhenItCannotSayWhere: a root that cannot be made absolute
// fails the cleanup, before anything is removed, rather than report a path
// relative to nothing. The failure is handed in through cleanup's parameter,
// not a package variable, so the test is safe to run in parallel.
func TestCleanupFailsWhenItCannotSayWhere(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))
	failing := func(string) (string, error) { return "", errors.New("getwd: no such file or directory") }

	_, err := cleanup([]string{dir}, failing)

	require.ErrorContains(t, err, "resolving")
	require.FileExists(t, filepath.Join(dir, artifactRelPath), "nothing is removed")
}

// TestCleanupReportsAbsolutePaths: a relative root, the default "." among
// them, is reported absolute, and so is what was removed under it.
func TestCleanupReportsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	writeDbtProject(t, dir)
	require.NoError(t, PreDeploy(dir))
	t.Chdir(dir)
	wd, err := os.Getwd()
	require.NoError(t, err)

	report, err := Cleanup()

	require.NoError(t, err)
	require.Equal(t, []string{wd}, report.Roots)
	require.Equal(t, []string{filepath.Join(resolved(t, wd), artifactRelPath)}, report.Removed)
}
