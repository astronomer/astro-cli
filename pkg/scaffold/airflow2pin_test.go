package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/runtimeversions"
)

// runtimeAirflowFrom answers, for a conversion, the Airflow each Airflow 2
// build in builds carries, and records what it was asked.
func runtimeAirflowFrom(builds map[string]string, asked *[]string) func(string) string {
	return func(build string) string {
		*asked = append(*asked, build)
		return builds[build]
	}
}

func initWithRuntimeAirflow(t *testing.T, files map[string]string, runtimeAirflow func(string) string) (string, *Result, *manifest.Manifest) {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, files)
	res, err := Run(dir, Options{RuntimeAirflow: runtimeAirflow})
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	m, err := manifest.Parse(data)
	require.NoError(t, err)
	return dir, res, m
}

func hasMinorNote(res *Result) bool {
	return strings.Contains(strings.Join(res.Notes, "\n"), "does not name the Airflow minor")
}

// A 1.x Dockerfile on one Airflow 2 build pins the Airflow that build
// carries: runtime 12.6.0 runs 2.10.4, so the requirement is 2.10.*, not the
// newest Airflow 2, which the lockfile and standalone would otherwise run
// beside an image that runs 2.10.4.
func TestConversionPinsTheAirflowAnAirflow2BuildCarries(t *testing.T) {
	builds := map[string]string{"12.6.0": "2.10.4"}

	t.Run("a Dockerfile that is the build", func(t *testing.T) {
		var asked []string
		_, res, m := initWithRuntimeAirflow(t, map[string]string{
			fileDockerfile:     "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN echo hi\n",
			"requirements.txt": "pandas>=2\n",
		}, runtimeAirflowFrom(builds, &asked))

		assert.Equal(t, []string{"12.6.0"}, asked)
		assert.Equal(t, "2.10", m.Airflow().Pin)
		assert.Contains(t, m.Project.Dependencies, "apache-airflow==2.10.*")
		assert.Equal(t, fileDockerfile, m.Astro.Dockerfile)
		assert.False(t, hasMinorNote(res), "the minor was read: %q", res.Notes)
	})

	t.Run("a pin-only Dockerfile, whose build is recorded", func(t *testing.T) {
		var asked []string
		dir, res, m := initWithRuntimeAirflow(t, map[string]string{
			fileDockerfile: "FROM quay.io/astronomer/astro-runtime:12.6.0\n",
		}, runtimeAirflowFrom(builds, &asked))

		assert.Equal(t, "2.10", m.Airflow().Pin)
		assert.Equal(t, "12.6.0", m.Astro.Runtime)
		assert.NoFileExists(t, filepath.Join(dir, fileDockerfile))
		assert.Empty(t, dockerfileNotes(res))
	})

	t.Run("a build on a mirror", func(t *testing.T) {
		var asked []string
		_, _, m := initWithRuntimeAirflow(t, map[string]string{
			fileDockerfile: "FROM registry.example.com/astro-runtime:12.6.0\nRUN echo hi\n",
		}, runtimeAirflowFrom(builds, &asked))
		assert.Equal(t, "2.10", m.Airflow().Pin)
	})

	// Not one build, or nothing to ask: the generation pin and its note, as
	// before.
	for name, tc := range map[string]struct {
		from           string
		runtimeAirflow func(string) string
	}{
		"a floating tag":       {"FROM quay.io/astronomer/astro-runtime:12\n", runtimeAirflowFrom(builds, new([]string))},
		"a flavor tag":         {"FROM quay.io/astronomer/astro-runtime:12.6.0-python-3.11\n", runtimeAirflowFrom(builds, new([]string))},
		"no catalog":           {"FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN echo hi\n", nil},
		"a build it lacks":     {"FROM quay.io/astronomer/astro-runtime:12.7.0\nRUN echo hi\n", runtimeAirflowFrom(builds, new([]string))},
		"an Airflow 3 runtime": {"FROM astrocrpublic.azurecr.io/runtime:3.3-11\nRUN echo hi\n", runtimeAirflowFrom(builds, new([]string))},
	} {
		t.Run(name, func(t *testing.T) {
			_, res, m := initWithRuntimeAirflow(t, map[string]string{fileDockerfile: tc.from}, tc.runtimeAirflow)
			if strings.Contains(tc.from, "runtime:3.3") {
				assert.Equal(t, "3.3", m.Airflow().Pin)
				return
			}
			assert.Equal(t, "2", m.Airflow().Pin)
			if !strings.Contains(tc.from, "python-3.11") && !strings.Contains(tc.from, ":12\n") {
				assert.True(t, hasMinorNote(res), "notes: %q", res.Notes)
			}
		})
	}
}

func airflow2Catalog(t *testing.T) func() *runtimeversions.Catalog {
	t.Helper()
	c, err := runtimeversions.Parse([]byte(`{"runtimeVersions": {
    "12.6.0": {"metadata": {"airflowVersion": "2.10.4", "channel": "stable", "releaseDate": "2024-12-01"}}
  }}`))
	require.NoError(t, err)
	return func() *runtimeversions.Catalog { return c }
}

// An Airflow 2 tag names only the generation, so the tag alone passes a 2.11
// requirement against a 2.10 image. With the catalog, the Airflow the build
// carries is held to the requirement too.
func TestCheckDockerfileAirflowReadsAnAirflow2BuildsReleaseFromTheCatalog(t *testing.T) {
	project := func(t *testing.T, pin string) (string, *manifest.Manifest) {
		t.Helper()
		dir := t.TempDir()
		writeTree(t, dir, map[string]string{
			fileDockerfile:  "FROM quay.io/astronomer/astro-runtime:12.6.0-base\nRUN echo hi\n",
			manifest.Marker: "[project]\nname = 'x'\ndependencies = ['apache-airflow==" + pin + "']\n\n[tool.astro]\ndockerfile = 'Dockerfile'\n",
		})
		data, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
		require.NoError(t, err)
		m, err := manifest.Parse(data)
		require.NoError(t, err)
		return dir, m
	}

	t.Run("another series is refused", func(t *testing.T) {
		dir, m := project(t, "2.11.*")
		err := CheckDockerfileAirflowWith(dir, m, airflow2Catalog(t))
		var verr *manifest.ValidationError
		require.ErrorAs(t, err, &verr)
		require.Len(t, verr.Problems, 1)
		assert.Equal(t, manifest.CodeDockerfileAirflowMismatch, verr.Problems[0].Code)
		assert.Contains(t, verr.Problems[0].Reason, "carries Airflow 2.10.4")
		assert.Contains(t, verr.Problems[0].Reason, "apache-airflow==2.10.*")
	})
	t.Run("its own series passes", func(t *testing.T) {
		dir, m := project(t, "2.10.*")
		assert.NoError(t, CheckDockerfileAirflowWith(dir, m, airflow2Catalog(t)))
	})
	t.Run("the generation passes", func(t *testing.T) {
		dir, m := project(t, "2.*")
		assert.NoError(t, CheckDockerfileAirflowWith(dir, m, airflow2Catalog(t)))
	})
	t.Run("without a catalog the tag alone decides", func(t *testing.T) {
		dir, m := project(t, "2.11.*")
		assert.NoError(t, CheckDockerfileAirflow(dir, m))
		assert.NoError(t, CheckDockerfileAirflowWith(dir, m, func() *runtimeversions.Catalog { return nil }))
	})
}
