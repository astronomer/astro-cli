package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const pinOnlyDockerfile = "FROM astrocrpublic.azurecr.io/runtime:3.1-12\n"

func writeAll(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
}

func runIn(t *testing.T, dir string, opts Options) *Changeset {
	t.Helper()
	cs, err := Plan(dir, opts)
	require.NoError(t, err)
	_, err = cs.Apply()
	require.NoError(t, err)
	return cs
}

// A file is retired only when what it said reached the manifest, and the notes
// are how that is known.
//
// Every case here deleted a file it should have kept before planRetirements
// moved after the notes. The first one is the worst: the run's own notes said
// the requirements were not carried, and the same run deleted the file holding
// them.
func TestRetireKeepsAFileNothingCarried(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		opts  Options
		keep  string
		why   string
	}{
		{
			name: "extras the generated Airflow requirement does not carry",
			files: map[string]string{
				"requirements.txt": "apache-airflow[celery,statsd]==2.9.1\npandas\n",
			},
			keep: "requirements.txt",
			why:  "the note tells the user to copy the extras out of this file, so it has to still exist",
		},
		{
			name: "the manifest's own specifier wins the merge",
			files: map[string]string{
				"pyproject.toml":   "[project]\nname = 'theirs'\nversion = '1.0.0'\ndependencies = ['pandas>=1.0']\n",
				"requirements.txt": "pandas==2.1.0\n",
			},
			keep: "requirements.txt",
			why:  "the pin the project installed with lost the merge, so this file is its only record",
		},
		{
			name: "two spellings of one distribution",
			files: map[string]string{
				"requirements.txt": "Flask==1.0\nflask==2.0\n",
			},
			keep: "requirements.txt",
			why:  "the dedup keeps one specifier; the other exists nowhere else",
		},
		{
			name:  "an --airflow-version outranks the Dockerfile tag",
			files: map[string]string{"Dockerfile": pinOnlyDockerfile},
			opts:  Options{AirflowVersion: "3.0"},
			keep:  "Dockerfile",
			why:   "the manifest pinned 3.0, so the 3.1 image this project built on is recorded only here",
		},
		{
			name: "an existing manifest pin outranks the Dockerfile tag",
			files: map[string]string{
				"pyproject.toml": "[project]\nname = 'theirs'\nversion = '1.0.0'\n" +
					"dependencies = ['apache-airflow==2.9.1']\n",
				"Dockerfile": pinOnlyDockerfile,
			},
			keep: "Dockerfile",
			why:  "the manifest says 2.9.1 and the image says 3.1; destroying the disagreement does not resolve it",
		},
		{
			name: "an instruction the note regex does not know",
			files: map[string]string{
				"Dockerfile": pinOnlyDockerfile + "MAINTAINER data-eng@example.com\n",
			},
			keep: "Dockerfile",
			why:  "MAINTAINER is in no allowlist, so only a positive test for pin-only keeps this file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, tc.files)
			cs := runIn(t, dir, tc.opts)

			assert.FileExists(t, filepath.Join(dir, tc.keep), tc.why)
			assert.NotContains(t, strings.Join(cs.Deleted, "\n"), tc.keep,
				"and it is not reported as deleted either")
		})
	}
}

// A Dockerfile that survives must not be left reading a file this run deleted.
func TestRetireKeepsWhatAKeptDockerfileReferences(t *testing.T) {
	for _, name := range []string{"requirements.txt", "packages.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeAll(t, dir, map[string]string{
				"Dockerfile":       pinOnlyDockerfile + "RUN pip install -r " + name + "\n",
				"requirements.txt": "pandas==2.1.0\n",
				"packages.txt":     "libpq-dev\n",
			})
			runIn(t, dir, Options{})

			assert.FileExists(t, filepath.Join(dir, "Dockerfile"), "it has a RUN, so it is build work")
			assert.FileExists(t, filepath.Join(dir, name),
				"the kept Dockerfile reads this file, and a build referencing a deleted file is worse than either outcome alone")
		})
	}
}

// The ordinary case still retires, or the change does nothing.
func TestRetireRemovesWhatWasCarriedWhole(t *testing.T) {
	dir := t.TempDir()
	writeAll(t, dir, map[string]string{
		"Dockerfile":       pinOnlyDockerfile,
		"requirements.txt": "pandas==2.1.0\n",
		"packages.txt":     "libpq-dev\n",
	})
	cs := runIn(t, dir, Options{})

	for _, name := range []string{"Dockerfile", "requirements.txt", "packages.txt"} {
		assert.NoFileExists(t, filepath.Join(dir, name), "%s was carried whole", name)
	}
	// Reported as deleted, not as updated: a caller rendering the two alike
	// tells the user a destroyed file was edited.
	assert.Len(t, cs.Deleted, 3)
	assert.NotContains(t, strings.Join(cs.Updated, "\n"), "requirements.txt")
}
