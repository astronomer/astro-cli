package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// An append has to land past the end of the array, whatever the array holds.
//
// Both append sites took len(asStrings(...)), which drops non-strings, so an
// array with one non-string entry reported a length one short and the append
// replaced the last element instead of following it.
func TestArrayLenCountsEveryElement(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    any
		want int
	}{
		{"all strings", []any{"a", "b"}, 2},
		{"a non-string among them", []any{"a", map[string]any{"name": "x"}}, 2},
		{"only a non-string", []any{int64(1)}, 1},
		{"empty", []any{}, 0},
		{"absent", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, arrayLen(tc.v))
		})
	}
}

// nonStringDeps is a manifest whose dependency list holds an entry this package
// cannot read. It is malformed for PEP 621, which requires strings, so it takes
// a hand-edited file to produce.
const nonStringDeps = "[project]\nname = 'x'\nversion = '1.0'\ndependencies = [\n" +
	"    'requests==2.31.0',\n    { name = 'weird' },\n]\n"

// A conversion must not destroy what it cannot read.
//
// It used to: the append index was one short, so the entry was overwritten by
// the Airflow pin, `astro init` reported success, and nothing on the hand-off
// list said a dependency had gone. Now the entry survives the edit, the
// manifest's own validation rejects it — dependencies must be strings — and the
// run refuses with the file exactly as its author left it.
func TestAdoptRefusesRatherThanOverwritingADependencyItCannotRead(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o755))
	path := filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(path, []byte(nonStringDeps), 0o600))

	_, err := Run(dir, Options{})
	require.Error(t, err, "a manifest this cannot carry must not be carried")
	// Cased as the Go field is, since that is what the decoder reports.
	assert.Contains(t, strings.ToLower(err.Error()), "dependencies",
		"the error names what it could not read")

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, nonStringDeps, string(raw),
		"the manifest is exactly as its author left it")
}

// The same guarantee on the other append site: merging a requirements.txt does
// not write over an entry it cannot read either.
func TestMergingRequirementsDoesNotOverwriteAnEntryItCannotRead(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o755))
	path := filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(path, []byte(nonStringDeps), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "requirements.txt"),
		[]byte("pandas>=2.0\n"), 0o600))

	_, err := Run(dir, Options{})
	require.Error(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, nonStringDeps, string(raw), "and nothing was written")
	// The file it was told to migrate is still there too, so nothing is lost
	// on either side of a refused run.
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
}

// The ordinary case is unaffected: a dependency list of strings still gains the
// Airflow pin at the end.
func TestAdoptStillAppendsToAnAllStringDependencyList(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o755))
	path := filepath.Join(dir, "pyproject.toml")
	require.NoError(t, os.WriteFile(path, []byte(
		"[project]\nname = 'x'\nversion = '1.0'\ndependencies = [\n    'requests==2.31.0',\n]\n",
	), 0o600))

	_, err := Run(dir, Options{})
	require.NoError(t, err)

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	got := string(raw)
	assert.Contains(t, got, "requests==2.31.0", "the existing dependency survives")
	assert.Contains(t, got, "apache-airflow", "and the pin was appended")
}
