package scaffold

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
)

// v2Project is an Astro project as an earlier astro converted it: a declared
// Dockerfile, and the manifest's lists beside whatever else files holds.
func v2Project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	tree := map[string]string{
		manifest.Marker: "[project]\nname = 'orders'\ndependencies = [\n    'apache-airflow==2.*',\n    'pandas==2.1.0',\n]\n\n" +
			"[tool.astro]\ndockerfile = 'Dockerfile'\npackages = ['libpq-dev']\n",
	}
	for k, v := range files {
		tree[k] = v
	}
	writeTree(t, dir, tree)
	return dir
}

func runUpdate(t *testing.T, dir string) *Result {
	t.Helper()
	res, err := Run(dir, Options{UpdateExisting: true})
	require.NoError(t, err)
	assert.True(t, res.Existing)
	return res
}

// Without UpdateExisting, an Astro project is refused as it always was, since
// Astro Desktop reads that refusal as "already converted".
func TestPlanStillRefusesAnAstroProjectByDefault(t *testing.T) {
	dir := v2Project(t, nil)
	_, err := Plan(dir, Options{})
	assert.True(t, errors.Is(err, ErrAlreadyAstroProject), "got %v", err)
}

// A Dockerfile an earlier conversion kept on its ONBUILD tag is moved onto
// -base, and the two lists it kept for that tag go, since every line of them
// is in the manifest. Nothing else in the project is touched.
func TestUpdateMovesAKeptDockerfileOntoBase(t *testing.T) {
	dir := v2Project(t, map[string]string{
		"Dockerfile":       "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN apt-get update\n",
		"requirements.txt": "pandas==2.1.0\n",
		"packages.txt":     "libpq-dev\n",
	})
	before, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)

	res := runUpdate(t, dir)

	got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "RUN apt-get update\n"), string(got))
	helper, err := os.ReadFile(filepath.Join(dir, airflowrt.InstallHelperPath))
	require.NoError(t, err)
	assert.Equal(t, airflowrt.InstallHelper(), helper)
	assert.NoFileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "packages.txt"))
	after, err := os.ReadFile(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after), "the manifest is not astro's to change here")
	assert.Len(t, res.Deleted, 2)
	assert.Contains(t, strings.Join(res.Updated, "\n"), "Dockerfile (now builds FROM quay.io/astronomer/astro-runtime:12.6.0-base")
	assert.Empty(t, res.Notes)
}

// A list that drifted from the manifest keeps the Dockerfile where it is: the
// ONBUILD tag still installs that line, and moving the file would drop it. The
// note names the line, and what the refusal will say until it moves.
func TestUpdateKeepsADockerfileWhoseListsDrifted(t *testing.T) {
	body := "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN apt-get update\n"
	dir := v2Project(t, map[string]string{
		"Dockerfile":       body,
		"requirements.txt": "pandas==2.1.0\nrequests==2.32.0\n",
		"packages.txt":     "libpq-dev\nunixodbc\n",
	})

	res := runUpdate(t, dir)

	got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, body, string(got))
	assert.FileExists(t, filepath.Join(dir, "requirements.txt"))
	assert.Empty(t, res.Updated)
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "requirements.txt: requests==2.32.0 is not in [project] dependencies")
	assert.Contains(t, notes, "packages.txt: unixodbc is not in packages under [tool.astro]")
	assert.Contains(t, notes, "Until it builds FROM a -base tag, astro local start, deploy and package refuse it")
}

// A Dockerfile already on -base gets the helper it runs: written when the
// project lacks it, refreshed when it is not this astro's, and left alone when
// it is. With nothing to do the run changes nothing.
func TestUpdateWritesAndRefreshesTheHelper(t *testing.T) {
	onBase := rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "RUN true\n")

	dir := v2Project(t, map[string]string{"Dockerfile": onBase})
	res := runUpdate(t, dir)
	assert.Contains(t, res.Created, airflowrt.InstallHelperPath+" (reads pyproject.toml and uv.lock inside the build)")
	assert.Contains(t, res.Created, fileDockerignore, "the build's context is the project, so it gets the ignore file a conversion writes")

	writeTree(t, dir, map[string]string{airflowrt.InstallHelperPath: "# an older helper\n"})
	res = runUpdate(t, dir)
	assert.Equal(t, []string{airflowrt.InstallHelperPath + " (refreshed to this astro's version)"}, res.Updated)
	helper, err := os.ReadFile(filepath.Join(dir, airflowrt.InstallHelperPath))
	require.NoError(t, err)
	assert.Equal(t, airflowrt.InstallHelper(), helper)

	res = runUpdate(t, dir)
	assert.Empty(t, res.Created)
	assert.Empty(t, res.Updated)
	assert.Empty(t, res.Deleted)
}

// A Dockerfile whose shape the rewrite will not take stays as it is, with the
// reason and what builds will do about it.
func TestUpdateNotesADockerfileItCannotMove(t *testing.T) {
	dir := v2Project(t, map[string]string{
		"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0 AS deps\nRUN true\nFROM quay.io/astronomer/astro-runtime:12.6.0\n",
	})
	res := runUpdate(t, dir)
	assert.Empty(t, res.Updated)
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "an earlier stage builds on quay.io/astronomer/astro-runtime:12.6.0")
	assert.True(t, strings.HasSuffix(res.Notes[0], "astro local start, deploy and package refuse it"))
}

func TestInstallHelperWarning(t *testing.T) {
	onBase := rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "RUN true\n")
	dir := v2Project(t, map[string]string{"Dockerfile": onBase})
	assert.Contains(t, InstallHelperWarning(dir, "Dockerfile"), "which this project does not have, so the build will fail. Run astro init to write it")

	writeTree(t, dir, map[string]string{airflowrt.InstallHelperPath: "# edited\n"})
	assert.Contains(t, InstallHelperWarning(dir, "Dockerfile"), "is not the one this astro writes. Run astro init to refresh it")

	writeTree(t, dir, map[string]string{airflowrt.InstallHelperPath: string(airflowrt.InstallHelper())})
	assert.Empty(t, InstallHelperWarning(dir, "Dockerfile"))

	other := v2Project(t, map[string]string{"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0-base\nRUN pip install x\n"})
	assert.Empty(t, InstallHelperWarning(other, "Dockerfile"), "a Dockerfile that does not run the helper needs none")
	assert.Empty(t, InstallHelperWarning(other, ""), "no Dockerfile, no warning")
}

// CheckDockerfileOnBuild refuses a declared Dockerfile on an ONBUILD tag, and
// nothing else.
func TestCheckDockerfileOnBuild(t *testing.T) {
	dir := v2Project(t, map[string]string{"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN true\n"})
	m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
	require.NoError(t, err)
	require.ErrorIs(t, CheckDockerfileOnBuild(dir, m), airflowrt.ErrOnBuildBase)

	writeTree(t, dir, map[string]string{"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0-base\nRUN true\n"})
	require.NoError(t, CheckDockerfileOnBuild(dir, m))

	m.Astro.Dockerfile = ""
	require.NoError(t, CheckDockerfileOnBuild(dir, m))
}
