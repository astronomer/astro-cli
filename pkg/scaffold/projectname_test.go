package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A 1.x project keeps its own name.
//
// It states one in .astro/config.yaml, and a conversion used the directory's
// base name instead — so a project called orders-pipeline sitting in a
// directory called thedir became thedir. That is a rename nobody asked for,
// of the name the manifest is identified by: what `astro package` names its
// artifact after, what the env checklist reports against, and what the run
// itself prints back.
//
// Not the hostname, which comes from the project DIRECTORY
// (proxy.DeriveHostname) and never from the manifest.
func TestChooseNameTakesWhatTheProjectCallsItself(t *testing.T) {
	cases := []struct {
		name string
		// configName is what .astro/config.yaml states; empty omits the file.
		configName string
		optsName   string
		// dirName is the directory's base name, which is the last resort.
		dirName string

		want string
		// wantAdvisory is a fragment of what the run must say when the project
		// is not called what it said it was called. Empty means it should say
		// nothing, which is its own assertion: the ordinary case is silent.
		wantAdvisory string
	}{
		{
			name:       "the name the project states",
			configName: "orders-pipeline",
			dirName:    "thedir",
			want:       "orders-pipeline",
		},
		{
			name:     "the directory, when nothing states a name",
			dirName:  "thedir",
			want:     "thedir",
			optsName: "",
		},
		{
			// --name is the only one somebody typed on purpose.
			name:       "the flag, over everything",
			configName: "orders-pipeline",
			optsName:   "chosen-by-flag",
			dirName:    "thedir",
			want:       "chosen-by-flag",
		},
		{
			// A [project] name holds a restricted set of characters, so a
			// stated name can need respelling — and the respelling is
			// reported, because the project said what it was called and this
			// is not quite that.
			name:         "a stated name that is not a legal one",
			configName:   "Orders Pipeline",
			dirName:      "thedir",
			want:         "orders-pipeline",
			wantAdvisory: "from Orders Pipeline",
		},
		{
			// Nothing a [project] name can hold survives, so the directory is
			// used — and this is the loudest case, not the quietest: the
			// stated name is discarded rather than respelled.
			//
			// A name in a non-Latin script, which is the realistic way to
			// reach this. "!!!" was the first fixture here and was no use:
			// unquoted, YAML parses it to the empty string, so it never
			// reached the sanitizer and the case was a duplicate of "nothing
			// states a name" two rows above.
			name:         "a stated name with nothing a project name can hold",
			configName:   "数据管道",
			dirName:      "thedir",
			want:         "thedir",
			wantAdvisory: "has nothing a [project] name can hold",
		},
		{
			// The fallback's fallback: neither the file nor the directory
			// yields anything, and a manifest still needs a name.
			name:       "neither a stated name nor a usable directory",
			configName: "",
			dirName:    "...",
			want:       "astro-project",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), tc.dirName)
			require.NoError(t, os.MkdirAll(dir, 0o755))
			from1x := &project1x{}
			if tc.configName != "" {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
				require.NoError(t, os.WriteFile(
					filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
					[]byte("project:\n  name: "+tc.configName+"\n"), 0o600))
				read, err := read1xProject(dir)
				require.NoError(t, err)
				from1x = read
			}

			got, advisory := chooseName(dir, Options{Name: tc.optsName}, from1x)
			assert.Equal(t, tc.want, got)
			if tc.wantAdvisory == "" {
				assert.Empty(t, advisory, "a name carried as written needs no comment")
				return
			}
			assert.Contains(t, advisory, tc.wantAdvisory)
			assert.Contains(t, advisory, tc.want, "and the advisory says what it is called now")
		})
	}
}

// Reading the name does not put the file up for deletion.
//
// .astro/config.yaml holds 1.x CLI configuration this conversion neither reads
// nor replaces, so the file stays. Recording it as one of the 1.x files present
// would offer it to planRetirements, which retires a file once everything it
// said reached the manifest — and the name would be everything, as far as that
// code could tell.
func TestReadingThe1xNameDoesNotRetireTheConfig(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
		[]byte("project:\n  name: orders-pipeline\n"), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	from1x, err := read1xProject(dir)
	require.NoError(t, err)
	require.Equal(t, "orders-pipeline", from1x.projectName)
	assert.NotContains(t, from1x.present, config1xRelPath,
		"the config is not a retirement candidate, so it must not be recorded as present")

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, "orders-pipeline", res.Name)
	_, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(config1xRelPath)))
	assert.NoError(t, statErr, "the config holds 1.x CLI configuration this run does not replace, so it stays")
}

// A malformed config costs the name, not the conversion.
//
// The 1.x CLI's own loader tolerated an unparseable config, the rest of a conversion
// does not depend on it, and failing `astro init` over YAML in a file being
// left behind anyway would be the least useful outcome available.
func TestAMalformed1xConfigDoesNotFailTheConversion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "thedir")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
		[]byte("project:\n  name: [not a string\n"), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	res, err := Run(dir, Options{})
	require.NoError(t, err, "a file this run only reads a name out of must not fail it")
	assert.Equal(t, "thedir", res.Name, "the name falls back to the directory")

	// And it says so. Silence here would be the same silent rename this whole
	// change is about, arrived at by a different route — and readAirflowSettings
	// already set the pattern: a file that will not parse is a blocker, not an
	// error.
	var found string
	for _, n := range res.Notes {
		if strings.Contains(n, config1xRelPath) && strings.Contains(n, "could not be read") {
			found = n
		}
	}
	assert.NotEmpty(t, found,
		"a config that will not parse costs the project its name, so the run has to say so: %v", res.Notes)
}

// Two files naming the project differently is worth one line.
//
// The manifest wins — it has already said what the project is called — but
// somebody who has only ever seen the 1.x name should not have to work out
// where it went.
func TestAdoptSaysWhenTheManifestNameDisagreesWithThe1xOne(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "thedir")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
		[]byte("project:\n  name: orders-pipeline\n"), 0o600))
	writeAll(t, dir, map[string]string{
		"pyproject.toml": "[project]\nname = 'internal-dags'\nversion = '0.1.0'\n" +
			"requires-python = '>=3.10'\ndependencies = []\n",
	})

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "internal-dags", res.Name, "the manifest's own name wins")

	var found string
	for _, a := range res.Advisories {
		if strings.Contains(a, "orders-pipeline") {
			found = a
		}
	}
	require.NotEmpty(t, found, "the 1.x name went somewhere and the run should say so: %v", res.Advisories)
	assert.Contains(t, found, "internal-dags", "and name what it kept instead")
}

// An adopted manifest's own name wins, and one without a name takes the 1.x
// project's.
//
// Two halves of the same rule: a manifest that states a name has said what the
// project is called, and a repo that was never a Python package often has no
// [project] table at all.
func TestAdoptKeepsAManifestNameAndFillsAMissingOne(t *testing.T) {
	t.Run("a stated manifest name is kept", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "thedir")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
			[]byte("project:\n  name: orders-pipeline\n"), 0o600))
		writeAll(t, dir, map[string]string{
			"pyproject.toml": "[project]\nname = 'kept-by-manifest'\nversion = '0.1.0'\n" +
				"requires-python = '>=3.10'\ndependencies = []\n",
		})

		res, err := Run(dir, Options{})
		require.NoError(t, err)
		assert.Equal(t, "kept-by-manifest", res.Name)
	})

	t.Run("a manifest with no name takes the 1.x one", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "thedir")
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
			[]byte("project:\n  name: orders-pipeline\n"), 0o600))
		writeAll(t, dir, map[string]string{
			"pyproject.toml": "[tool.ruff]\nline-length = 100\n",
		})

		res, err := Run(dir, Options{})
		require.NoError(t, err)
		assert.Equal(t, "orders-pipeline", res.Name)
		manifest, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
		require.NoError(t, err)
		assert.Contains(t, string(manifest), "orders-pipeline")
		// The section it already had is still there.
		assert.Contains(t, string(manifest), "line-length = 100")
	})
}

// The rename is an advisory, not a note.
//
// Nothing is left to do — the project is already named that — and Result's doc
// draws the line there. A consumer rendering notes as "your remaining work"
// would otherwise tell somebody to go and rename a project that has been
// renamed.
func TestARewrittenNameIsAnAdvisory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "thedir")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".astro"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, filepath.FromSlash(config1xRelPath)),
		[]byte("project:\n  name: Orders Pipeline\n"), 0o600))
	writeAll(t, dir, map[string]string{"Dockerfile": pinOnlyDockerfile})

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	require.Equal(t, "orders-pipeline", res.Name)

	var found string
	for _, a := range res.Advisories {
		if strings.Contains(a, "Orders Pipeline") {
			found = a
		}
	}
	require.NotEmpty(t, found, "a rewritten name should be reported: %v", res.Advisories)
	assert.Contains(t, found, "orders-pipeline", "and should say what it is called now")
	for _, n := range res.Notes {
		assert.NotContains(t, n, "Orders Pipeline",
			"the rename has happened, so it is not work left to do")
	}
}
