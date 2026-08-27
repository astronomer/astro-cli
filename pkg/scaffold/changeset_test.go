package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// snapshot records every path under dir with its type, mode and contents, so a
// test can prove nothing changed rather than checking the handful of files it
// happened to think of.
//
// Modes and link targets are in it deliberately. An earlier version recorded
// only paths and bytes, which would have passed a Plan that chmod'ed a file or
// replaced a symlink with a regular file of identical content — and it read
// symlinks through, so a dangling one aborted the walk with a helper error
// rather than a behavioral one.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			return relErr
		}
		info, infoErr := d.Info() // Lstat semantics: describes the link, not its target.
		if infoErr != nil {
			return infoErr
		}
		switch {
		case d.IsDir():
			out[rel+"/"] = fmt.Sprintf("dir %v", info.Mode().Perm())
		case info.Mode()&os.ModeSymlink != 0:
			target, linkErr := os.Readlink(path)
			if linkErr != nil {
				return linkErr
			}
			out[rel] = "symlink -> " + target
		default:
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			out[rel] = fmt.Sprintf("file %v %s", info.Mode().Perm(), data)
		}
		return nil
	})
	require.NoError(t, err)
	return out
}

// The property the split exists for. Astro Desktop shows this change set to a
// person before anything lands, so a Plan that touched the project would make
// the preview a report of something that already happened.
func TestPlanWritesNothing(t *testing.T) {
	t.Run("greenfield", func(t *testing.T) {
		dir := t.TempDir()
		before := snapshot(t, dir)

		_, err := Plan(dir, Options{})
		require.NoError(t, err)

		assert.Equal(t, before, snapshot(t, dir), "Plan modified the project")
	})

	t.Run("adopting an existing project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
			[]byte("[project]\nname = \"demo\"\nversion = \"0.1.0\"\n"), 0o600))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist\n"), 0o600))
		require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o750))
		before := snapshot(t, dir)

		cs, err := Plan(dir, Options{})
		require.NoError(t, err)
		// It planned real work — otherwise this would pass by doing nothing.
		require.NotEmpty(t, cs.Changes)

		assert.Equal(t, before, snapshot(t, dir), "Plan modified the project")
	})

	// Run creates the project directory itself, so Plan must not.
	t.Run("a directory that does not exist yet", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "new-project")

		_, err := Plan(dir, Options{})
		require.NoError(t, err)

		_, statErr := os.Stat(dir)
		assert.Error(t, statErr, "Plan created the project directory")
	})
}

// The manifest is written last so a run that dies part-way through is safe to
// repeat: a manifest carrying [tool.astro] is the one thing that makes a rerun
// refuse. Apply walks the slice in order, so the order is the guarantee.
func TestPlanPutsTheManifestLast(t *testing.T) {
	dir := t.TempDir()
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	require.NotEmpty(t, cs.Changes)

	last := cs.Changes[len(cs.Changes)-1]
	assert.Equal(t, "pyproject.toml", last.Path)

	for i, c := range cs.Changes[:len(cs.Changes)-1] {
		assert.NotEqualf(t, "pyproject.toml", c.Path, "manifest also planned at index %d", i)
	}
}

// Apply performs what Plan decided, so the two together are what Run always did.
func TestPlanThenApplyProducesAWorkingProject(t *testing.T) {
	dir := t.TempDir()

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)

	for _, want := range []string{"pyproject.toml", ".gitignore", "AGENTS.md"} {
		_, statErr := os.Stat(filepath.Join(dir, want))
		assert.NoErrorf(t, statErr, "%s was planned but not applied", want)
	}
	for _, want := range []string{"dags", "include", "plugins", "tests"} {
		info, statErr := os.Stat(filepath.Join(dir, want))
		require.NoErrorf(t, statErr, "%s/ was planned but not applied", want)
		assert.True(t, info.IsDir())
	}
	assert.Equal(t, dir, res.Dir)
	assert.NotEmpty(t, res.Created)
}

// Run is Plan plus Apply, so it has to keep reporting exactly what it did
// before the split.
func TestRunStillReportsWhatItDid(t *testing.T) {
	dir := t.TempDir()

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	assert.False(t, res.Adopted)
	assert.Contains(t, res.Created, "pyproject.toml")
	assert.Contains(t, res.Created, "dags/")
	if runtime.GOOS != windowsOS {
		assert.Contains(t, res.Created, "CLAUDE.md -> AGENTS.md")
	}
}

// The preview is only as good as the bytes in it: a caller diffs Change.Content
// against what is on disk, so Content has to be what actually lands, not a
// description of an edit.
func TestPlannedContentIsWhatLands(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname = \"demo\"\nversion = \"0.1.0\"\n"), 0o600))
	// An existing .gitignore with no .env rule is the case that gets edited
	// rather than created.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("dist\n"), 0o600))

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)

	planned := map[string]string{}
	for _, c := range cs.Changes {
		if c.Kind == CreateFile || c.Kind == UpdateFile {
			planned[c.Path] = string(c.Content)
		}
	}
	require.Contains(t, planned, ".gitignore", "the .env rule was not planned as an edit")
	require.Contains(t, planned, "pyproject.toml")

	_, err = cs.Apply()
	require.NoError(t, err)

	for path, want := range planned {
		got, readErr := os.ReadFile(filepath.Join(dir, path))
		require.NoError(t, readErr)
		assert.Equalf(t, want, string(got), "%s on disk differs from what the plan showed", path)
	}
}

// An edit keeps the user's file rather than replacing it, so a .gitignore the
// repo already had keeps its mode. Unix only: Windows has no Unix permission
// bits, and Go reports 0666 for any writable file there.
func TestApplyEditsInPlace(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("file modes are not Unix permission bits on Windows")
	}
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("dist\n"), 0o600))

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)

	info, err := os.Stat(gitignore)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the user's file was republished with a different mode")
}

// Every change carries a label, because the labels are what a person reads and
// they are not derivable from the path: a directory reads "dags/" and a symlink
// reads "CLAUDE.md -> AGENTS.md".
func TestEveryChangeIsLabelled(t *testing.T) {
	// Both paths, because they build the manifest change differently and only
	// the greenfield one was covered — which is exactly how the adopted
	// manifest, the most important line in a conversion preview, came to render
	// blank.
	t.Run("greenfield", func(t *testing.T) {
		assertAllLabelled(t, t.TempDir())
	})

	t.Run("adopting an existing project", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
			[]byte("[project]\nname = \"demo\"\nversion = \"0.1.0\"\n"), 0o600))
		assertAllLabelled(t, dir)
	})
}

func assertAllLabelled(t *testing.T, dir string) {
	t.Helper()
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	require.NotEmpty(t, cs.Changes)

	var unlabelled []string
	for _, c := range cs.Changes {
		if len(c.Labels) == 0 {
			unlabelled = append(unlabelled, string(c.Kind)+" "+c.Path)
		}
	}
	sort.Strings(unlabelled)
	assert.Empty(t, unlabelled, "changes with no label to show a person")
}

// Changeset is exported and can be built by hand or decoded from a UI, so Apply
// has to refuse what Plan would never produce. Each of these was a silent
// success before: an escaping path wrote outside the directory the user
// approved, an unknown kind did nothing and reported done, and a write with no
// content truncated the user's file to zero bytes.
func TestApplyRefusesWhatPlanWouldNeverProduce(t *testing.T) {
	content := []byte("x")
	for _, tc := range []struct {
		name string
		c    Change
	}{
		{"a path that climbs out", Change{Kind: CreateFile, Path: "../escaped.txt", Content: content}},
		{"an absolute path", Change{Kind: CreateFile, Path: "/tmp/escaped.txt", Content: content}},
		{"the project directory itself", Change{Kind: Delete, Path: ".."}},
		{"no path at all", Change{Kind: CreateFile, Path: "", Content: content}},
		{"a kind nothing knows", Change{Kind: "bogus", Path: "f.txt", Content: content}},
		{"a create with no content", Change{Kind: CreateFile, Path: "f.txt"}},
		{"an update with no content", Change{Kind: UpdateFile, Path: "f.txt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cs := &Changeset{Result: Result{Dir: dir}, Changes: []Change{tc.c}}
			_, err := cs.Apply()
			require.Error(t, err)
		})
	}
}

// A climbing path must not write outside the project even by accident, so this
// checks the filesystem rather than only the error.
func TestApplyDoesNotWriteOutsideTheProject(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "project")
	require.NoError(t, os.Mkdir(dir, 0o750))

	cs := &Changeset{Result: Result{Dir: dir}, Changes: []Change{
		{Kind: CreateFile, Path: "../escaped.txt", Content: []byte("x")},
	}}
	_, err := cs.Apply()
	require.Error(t, err)

	_, statErr := os.Stat(filepath.Join(parent, "escaped.txt"))
	assert.Error(t, statErr, "a change escaped the project directory")
}

// An update edits a file that is there. If the user deletes it while reviewing
// the preview, resurrecting it — with their old content, at the wrong mode — is
// worse than refusing.
func TestApplyRefusesAnUpdateWhoseFileWentAway(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	require.NoError(t, os.WriteFile(gitignore, []byte("dist\n"), 0o600))

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	require.NoError(t, os.Remove(gitignore))

	_, err = cs.Apply()
	require.ErrorIs(t, err, ErrChangedOnDisk)
	_, statErr := os.Stat(gitignore)
	assert.Error(t, statErr, "the file the user deleted came back")
}

// The window between Plan and Apply is human-scale by design, so a directory
// appearing in it is ordinary. Aborting half-applied would leave the project
// without its manifest and the caller without a record of what landed.
func TestApplyToleratesWorkDoneWhileReviewing(t *testing.T) {
	dir := t.TempDir()

	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	// The user makes dags/ themselves while deciding.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o750))

	_, err = cs.Apply()
	require.NoError(t, err)
	_, statErr := os.Stat(filepath.Join(dir, "pyproject.toml"))
	assert.NoError(t, statErr, "the run died before the manifest")
}

// The Result must not share backing arrays with the Changeset the caller still
// holds, or an append to either can overwrite the other.
func TestApplyReturnsACopy(t *testing.T) {
	dir := t.TempDir()
	cs, err := Plan(dir, Options{})
	require.NoError(t, err)
	res, err := cs.Apply()
	require.NoError(t, err)
	require.NotEmpty(t, res.Created)

	before := append([]string(nil), cs.Created...)
	res.Created = append(res.Created, "appended by the caller")
	assert.Equal(t, before, cs.Created, "the Result shares its slices with the Changeset")
}
