package scaffold

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/envschema"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// editFixture is a manifest a person wrote: a comment at the top, one beside a
// value, and a link under its own header rather than as an inline table.
const editFixture = `# the orders team's project
[project]
name = 'orders'
dependencies = ['apache-airflow==3.1.*']

[tool.astro]
packages = ['libpq-dev'] # pinned on purpose

[tool.astro.deployments.prod]
url = 'https://airflow.example.com'
auth = { method = 'none' }
`

func writeEditFixture(t *testing.T, body string, mode fs.FileMode) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	require.NoError(t, os.Chmod(path, mode))
	return dir, path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func setKey(key []string, value any) ManifestEdit {
	return func(_ *manifest.Manifest, ed tomledit.Editor) error {
		return ed.Set(key, value)
	}
}

func TestEditManifestEditsInPlace(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)

	require.NoError(t, EditManifest(dir, nil, setKey([]string{"tool", "astro", "env", "REGION"}, map[string]any{})))

	got := readFile(t, path)
	assert.Contains(t, got, "# the orders team's project")
	assert.Contains(t, got, "# pinned on purpose")
	assert.Contains(t, got, "[tool.astro.deployments.prod]")
	m, err := manifest.Parse([]byte(got))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	assert.Contains(t, s.EnvVars, "REGION")
}

// A result manifest.Parse refuses is not written: the parser refuses the whole
// file over one bad key, so writing it would stop every other link loading.
func TestEditManifestRefusesAResultThatDoesNotParse(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)

	// A domain with no workspace and no astro-auth link names a host for
	// nothing, which the parser refuses. Nothing short of the parser knows that.
	err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "domain"}, "astronomer.io"))

	require.ErrorIs(t, err, ErrEditRefused)
	var invalid *manifest.ValidationError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, manifest.CodeDomainWithoutWorkspace, invalid.Problems[0].Code)
	assert.Equal(t, editFixture, readFile(t, path), "a refused edit changed the file")
}

// manifest.Parse leaves [tool.astro.env] untyped, so an env write that only
// passed it could save a section `astro local start` then refuses.
func TestEditManifestRefusesAnEnvSectionTheSchemaParserRejects(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)

	err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "env", "MY-VAR"}, map[string]any{}))

	require.ErrorIs(t, err, ErrEditRefused)
	var schema *envschema.SchemaError
	require.ErrorAs(t, err, &schema)
	assert.Equal(t, editFixture, readFile(t, path), "a refused edit changed the file")
}

// The env round trip judges what the edit did to the section. A section that
// was broken before the edit and is untouched by it does not block an
// unrelated write, and an edit that does touch it is still held to the parser.
func TestEditManifestJudgesOnlyAnEnvSectionTheEditChanged(t *testing.T) {
	broken := editFixture + "\n[tool.astro.env]\nMY-VAR = {}\n"

	t.Run("untouched", func(t *testing.T) {
		dir, path := writeEditFixture(t, broken, 0o644)
		require.NoError(t, EditManifest(dir, nil, setKey([]string{"tool", "astro", "deployments", "prod", "default"}, true)))
		assert.Contains(t, readFile(t, path), "default = true")
	})
	t.Run("changed", func(t *testing.T) {
		dir, path := writeEditFixture(t, broken, 0o644)
		err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "env", "REGION"}, map[string]any{}))
		require.ErrorIs(t, err, ErrEditRefused)
		assert.Equal(t, broken, readFile(t, path))
	})
}

// An edit that writes through the manifest it was handed changes before's maps
// too. The untouched-section comparison must not see that as the file's own
// state, or the new declaration would skip the schema parse.
func TestEditManifestJudgesTheEnvSectionAgainstTheFileNotTheEditsCopy(t *testing.T) {
	body := editFixture + "\n[tool.astro.env]\nREGION = {}\n"
	dir, path := writeEditFixture(t, body, 0o644)

	err := EditManifest(dir, nil, func(before *manifest.Manifest, ed tomledit.Editor) error {
		env := before.Astro.Env
		env["BAD-NAME"] = map[string]any{}
		return ed.Set([]string{"tool", "astro", "env", "BAD-NAME"}, env["BAD-NAME"])
	})

	require.ErrorIs(t, err, ErrEditRefused)
	var schema *envschema.SchemaError
	require.ErrorAs(t, err, &schema)
	assert.Equal(t, body, readFile(t, path))
}

func TestEditManifestKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no group and other permission bits to keep")
	}
	for _, mode := range []fs.FileMode{0o600, 0o644, 0o640} {
		t.Run(mode.String(), func(t *testing.T) {
			dir, path := writeEditFixture(t, editFixture, mode)
			require.NoError(t, EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev")))
			info, err := os.Stat(path)
			require.NoError(t, err)
			assert.Equal(t, mode, info.Mode().Perm())
			assert.Contains(t, readFile(t, path), "dockerfile = 'Dockerfile.dev'")
		})
	}
}

// A rename replaces a file whatever its own permission bits say.
func TestEditManifestRefusesAReadOnlyFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may write a read-only file, so the check lets it through")
	}
	dir, path := writeEditFixture(t, editFixture, 0o444)
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "read-only")
	assert.Equal(t, editFixture, readFile(t, path))
}

func TestEditManifestWritesThroughASymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symlink needs developer mode on Windows")
	}
	shared := filepath.Join(t.TempDir(), "shared.toml")
	require.NoError(t, os.WriteFile(shared, []byte(editFixture), 0o644))
	dir := t.TempDir()
	link := filepath.Join(dir, manifest.Marker)
	require.NoError(t, os.Symlink(shared, link))

	require.NoError(t, EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev")))

	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.Equal(t, fs.ModeSymlink, info.Mode()&fs.ModeSymlink, "the write replaced the symlink with a file")
	assert.Contains(t, readFile(t, shared), "dockerfile = 'Dockerfile.dev'")
}

func TestEditManifestNeverCreatesAManifest(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		dir := t.TempDir()
		err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))
		require.ErrorIs(t, err, manifest.ErrNotFound)
		require.ErrorIs(t, err, fs.ErrNotExist)
		assert.NoFileExists(t, filepath.Join(dir, manifest.Marker))
	})
	t.Run("not an astro project", func(t *testing.T) {
		body := "[project]\nname = 'someone-else'\n"
		dir, path := writeEditFixture(t, body, 0o644)
		err := EditManifest(dir, nil, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))
		require.ErrorIs(t, err, manifest.ErrNoAstroSection)
		assert.Equal(t, body, readFile(t, path))
	})
}

// An edit that changes nothing does not touch the file, so an idempotent
// declare does not wake whatever watches the manifest.
func TestEditManifestWritesNothingForANoOpEdit(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, os.Chtimes(path, past, past))

	require.NoError(t, EditManifest(dir, nil, setKey([]string{"tool", "astro", "packages"}, []any{"libpq-dev"})))

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.True(t, info.ModTime().Equal(past), "a no-op edit rewrote the file")
}

func TestEditManifestPassesOnTheEditError(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)
	boom := errors.New("boom")

	err := EditManifest(dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		require.NoError(t, ed.Set([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))
		return boom
	})

	require.ErrorIs(t, err, boom)
	assert.Equal(t, editFixture, readFile(t, path), "an abandoned edit was written")
}

// The wrapper spans the read, not just the write. Here the wrapper plays the
// other writer that held the lock first: it changes the file before letting
// this edit run. An edit that read before the wrapper would write back the old
// file with its own change on top, and the other writer's edit would be gone.
func TestEditManifestReadsInsideTheWrapper(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)
	var events []string

	wrap := func(run func() error) error {
		events = append(events, "lock")
		other := strings.Replace(editFixture, "apache-airflow==3.1.*", "apache-airflow==3.0.*", 1)
		require.NoError(t, os.WriteFile(path, []byte(other), 0o644))
		err := run()
		events = append(events, "unlock")
		return err
	}
	err := EditManifest(dir, wrap, func(before *manifest.Manifest, ed tomledit.Editor) error {
		events = append(events, "edit read airflow "+before.Airflow().Pin)
		return ed.Set([]string{"tool", "astro", "env", "REGION"}, map[string]any{})
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"lock", "edit read airflow 3.0", "unlock"}, events)
	got := readFile(t, path)
	assert.Contains(t, got, "apache-airflow==3.0.*", "the other writer's edit was lost")
	assert.Contains(t, got, "REGION")
}

func TestEditManifestReportsAWrapperThatNeverRan(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)

	err := EditManifest(dir, func(func() error) error { return nil }, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))

	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrapper")
	assert.Equal(t, editFixture, readFile(t, path))
}

func TestEditManifestReturnsTheWrapperError(t *testing.T) {
	dir, _ := writeEditFixture(t, editFixture, 0o644)
	held := errors.New("watcher could not be held")

	err := EditManifest(dir, func(func() error) error { return held }, setKey([]string{"tool", "astro", "dockerfile"}, "Dockerfile.dev"))

	require.ErrorIs(t, err, held)
}

// Writers sharing one lock through the wrapper all land. Each one's
// read-modify-write is whole, so none of them reads a file another is about to
// replace.
func TestEditManifestConcurrentWritersThroughASharedLockAllLand(t *testing.T) {
	dir, path := writeEditFixture(t, editFixture, 0o644)
	var mu sync.Mutex
	locked := func(run func() error) error {
		mu.Lock()
		defer mu.Unlock()
		return run()
	}

	const writers = 16
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs <- EditManifest(dir, locked, setKey([]string{"tool", "astro", "env", fmt.Sprintf("VAR_%02d", i)}, map[string]any{}))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	s, err := envschema.ParseSchema(m.Astro.Env)
	require.NoError(t, err)
	assert.Len(t, s.EnvVars, writers)
}

// tomledit refuses to Set over a table, and a link a person wrote under its own
// header is one. ReplaceTable is what makes "replace this link" work there.
func TestReplaceTableReplacesAHeaderTable(t *testing.T) {
	key := []string{"tool", "astro", "deployments", "prod"}
	value := map[string]any{"url": "https://other.example.com", "auth": map[string]any{"method": "none"}}

	ed, err := tomledit.NewSurgical([]byte(editFixture))
	require.NoError(t, err)
	require.Error(t, ed.Set(key, value), "tomledit now sets over a table; ReplaceTable's reason is gone")

	dir, path := writeEditFixture(t, editFixture, 0o644)
	require.NoError(t, EditManifest(dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		return ReplaceTable(ed, key, value)
	}))
	m, err := manifest.Parse([]byte(readFile(t, path)))
	require.NoError(t, err)
	assert.Equal(t, "https://other.example.com", m.Astro.Deployments["prod"].URL)
}
