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
// Lines a conversion never carries, an Airflow requirement (the pin's) and a
// second line for a distribution already listed, are not drift: they could
// never be in the manifest as written, so they do not keep the file on its
// ONBUILD tag.
func TestUpdateIgnoresLinesAConversionDoesNotCarry(t *testing.T) {
	dir := v2Project(t, map[string]string{
		"Dockerfile":       "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN true\n",
		"requirements.txt": "apache-airflow==2.10.0\npandas==2.1.0\npandas>=2\n",
	})
	res := runUpdate(t, dir)
	assert.Contains(t, strings.Join(res.Updated, "\n"), "Dockerfile (now builds FROM quay.io/astronomer/astro-runtime:12.6.0-base")
	assert.Empty(t, res.Notes)
}

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
	assert.NoFileExists(t, filepath.Join(dir, staleHelperPath), "the CLI resolves the install; nothing is written for the build to run")
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
	assert.True(t, res.OnBuildKept, "the result says the Dockerfile stayed on its ONBUILD tag")
	notes := strings.Join(res.Notes, "\n")
	assert.Contains(t, notes, "requirements.txt: requests==2.32.0 is not in [project] dependencies")
	assert.Contains(t, notes, "packages.txt: unixodbc is not in packages under [tool.astro]")
	assert.Contains(t, notes, "Until it builds FROM a -base tag, each build of it warns")
}

// previewDockerfile is a Dockerfile as an Astro CLI preview moved it onto
// -base: an earlier stage of the project's own, the install block that ran
// the helper in a stage of its own, and the project's own steps.
const previewDockerfile = "FROM python:3.12-slim AS tools\nRUN true\n" +
	"# Installs this project's packages from pyproject.toml and uv.lock\n" +
	"# (.astro/install-project.py reads them; astro init keeps both up to date).\n" +
	"ARG ASTRO_UV_IMAGE=ghcr.io/astral-sh/uv:0.11.8\n" +
	"FROM ${ASTRO_UV_IMAGE} AS astro-uv\n" +
	"FROM quay.io/astronomer/astro-runtime:12.6.0-base AS astro-install\n" +
	"USER root\n" +
	"COPY pyproject.toml uv.lock* .astro/install-project.py /tmp/astro-project/\n" +
	"COPY --from=astro-uv /uv /opt/astro-uv/uv\n" +
	"RUN python /tmp/astro-project/install-project.py --project /tmp/astro-project --out /tmp/astro-install\n" +
	"FROM --platform=linux/amd64 quay.io/astronomer/astro-runtime:12.6.0-base AS app\n" +
	"COPY --from=astro-install /tmp/astro-install/packages.txt .\n" +
	"USER astro\n" +
	"COPY --chown=astro:0 . .\n" +
	"\n" + ownStepsComment + "COPY --from=tools /usr/bin/true /tmp/true\n"

// previewHelper is the helper those previews wrote, by its first line.
const previewHelper = staleHelperHeader + "\n# astro-install-helper: 5\nprint('installing')\n"

// A Dockerfile a preview moved onto -base gets this astro's install block in
// place of the one that ran the helper, keeping the stages before it, the
// final stage's flags and name, and its own steps; the helper goes. A second
// run changes nothing.
func TestUpdateReplacesAPreviewInstall(t *testing.T) {
	dir := v2Project(t, map[string]string{"Dockerfile": previewDockerfile, staleHelperPath: previewHelper})

	res := runUpdate(t, dir)

	got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, "FROM python:3.12-slim AS tools\nRUN true\n"+installBlockComment+
		airflowrt.InstallDockerfileWith("--platform=linux/amd64 quay.io/astronomer/astro-runtime:12.6.0-base",
			airflowrt.InstallOptions{Alias: "app", ListsFrom: airflowrt.InstallContext})+
		"\n"+ownStepsComment+"COPY --from=tools /usr/bin/true /tmp/true\n", string(got))
	assert.NoFileExists(t, filepath.Join(dir, staleHelperPath))
	assert.Contains(t, strings.Join(res.Updated, "\n"), "Dockerfile (install steps replaced with the ones this astro writes")
	assert.Contains(t, strings.Join(res.Deleted, "\n"), staleHelperPath)
	assert.Contains(t, res.Created, fileDockerignore, "the build's context is the project, so it gets the ignore file a conversion writes")
	assert.False(t, res.OnBuildKept)
	assert.Empty(t, res.Notes)
	assert.Empty(t, StaleHelperWarning(dir, "Dockerfile"))

	res = runUpdate(t, dir)
	assert.Empty(t, res.Created)
	assert.Empty(t, res.Updated)
	assert.Empty(t, res.Deleted)
}

// olderInstall is the install block an earlier astro wrote: the lists it read
// before the routed install's two files.
func olderInstall(from string) string {
	return strings.NewReplacer(
		" "+airflowrt.RoutedName+" "+airflowrt.RoutedLinksName+" ./", " ./",
		"if [ -s "+airflowrt.RoutedName+" ]; then pip install --no-cache-dir --root-user-action=ignore --no-index --no-deps --find-links "+airflowrt.RoutedLinksName+" -r "+airflowrt.RoutedName+"; fi && ", "",
	).Replace(airflowrt.InstallDockerfileWith(from, airflowrt.InstallOptions{ListsFrom: airflowrt.InstallContext}))
}

// A Dockerfile whose install steps an older astro wrote gets this astro's,
// and only those: the stages before them and the file's own steps after them
// stay, their comment or not. A second run changes nothing.
func TestUpdateRefreshesAnOlderInstallBlock(t *testing.T) {
	const from = "quay.io/astronomer/astro-runtime:12.6.0-base"
	older := olderInstall(from)
	require.NotEqual(t, airflowrt.InstallDockerfile(from), older)
	for name, own := range map[string]string{
		"with its comment":    "\n" + ownStepsComment + "ENV TZ=UTC\n",
		"without its comment": "RUN echo ours\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := v2Project(t, map[string]string{"Dockerfile": "FROM python:3.12-slim AS tools\n" + installBlockComment + older + own})

			res := runUpdate(t, dir)

			got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
			require.NoError(t, err)
			assert.Equal(t, "FROM python:3.12-slim AS tools\n"+installBlockComment+airflowrt.InstallDockerfile(from)+own, string(got))
			assert.Contains(t, strings.Join(res.Updated, "\n"), "Dockerfile (install steps updated to the ones this astro writes)")
			assert.Empty(t, res.Notes)

			res = runUpdate(t, dir)
			assert.Empty(t, res.Updated)
		})
	}

	// A block whose last line is gone is not one this can tell the end of.
	edited := installBlockComment + strings.Replace(older, "COPY --chown=astro:0 . .\n", "COPY . .\n", 1)
	dir := v2Project(t, map[string]string{"Dockerfile": edited})
	res := runUpdate(t, dir)
	assert.NotContains(t, strings.Join(res.Updated, "\n"), "install steps updated")
	got, err := os.ReadFile(filepath.Join(dir, "Dockerfile"))
	require.NoError(t, err)
	assert.Equal(t, edited, string(got))
}

// A preview's Dockerfile whose block cannot be found (someone rewrote the
// comment) is left as it is, and so is the helper it still runs, with a note.
// A helper no Dockerfile runs goes, and a file of the project's own at that
// path stays.
func TestUpdateLeavesAHelperTheDockerfileStillRuns(t *testing.T) {
	edited := strings.Replace(previewDockerfile, "# Installs this project's packages", "# Our install", 1)
	dir := v2Project(t, map[string]string{"Dockerfile": edited, staleHelperPath: previewHelper})
	res := runUpdate(t, dir)
	assert.Empty(t, res.Updated)
	assert.Empty(t, res.Deleted)
	assert.FileExists(t, filepath.Join(dir, staleHelperPath))
	require.Len(t, res.Notes, 1)
	assert.Contains(t, res.Notes[0], "cannot find the install steps it wrote around it")

	onBase := rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "RUN true\n")
	dir = v2Project(t, map[string]string{"Dockerfile": onBase, staleHelperPath: previewHelper})
	res = runUpdate(t, dir)
	assert.Equal(t, []string{staleHelperPath + " (written by an earlier Astro CLI preview, removed: the CLI resolves the install before the build now)"}, res.Deleted)

	dir = v2Project(t, map[string]string{"Dockerfile": onBase, staleHelperPath: "print('ours')\n"})
	res = runUpdate(t, dir)
	assert.Empty(t, res.Deleted)
	assert.FileExists(t, filepath.Join(dir, staleHelperPath))
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
	assert.True(t, strings.HasSuffix(res.Notes[0], "rather than what pyproject.toml lists"))
}

func TestStaleHelperWarning(t *testing.T) {
	dir := v2Project(t, map[string]string{"Dockerfile": previewDockerfile, staleHelperPath: previewHelper})
	assert.Equal(t, "Dockerfile runs .astro/install-project.py, which an earlier Astro CLI preview wrote. "+
		"Run astro init to replace it with the install this astro resolves", StaleHelperWarning(dir, "Dockerfile"))

	other := v2Project(t, map[string]string{"Dockerfile": rewrittenDockerfile("quay.io/astronomer/astro-runtime:12.6.0-base", "RUN true\n")})
	assert.Empty(t, StaleHelperWarning(other, "Dockerfile"), "a Dockerfile that does not run the helper")
	assert.Empty(t, StaleHelperWarning(other, ""), "no Dockerfile, no warning")
}

// CheckDockerfileOnBuild refuses a Dockerfile on an ONBUILD tag only when
// astro init would move it, so the refusal's "run astro init" always works.
// One a gate keeps on its tag builds as before, with the note init would give
// as the warning. One on -base, and no Dockerfile, pass without a word.
func TestCheckDockerfileOnBuild(t *testing.T) {
	onBuild := "FROM quay.io/astronomer/astro-runtime:12.6.0\nRUN true\n"
	load := func(dir string) *manifest.Manifest {
		t.Helper()
		m, err := manifest.Load(filepath.Join(dir, manifest.Marker))
		require.NoError(t, err)
		return m
	}

	t.Run("refused when init would move it", func(t *testing.T) {
		dir := v2Project(t, map[string]string{"Dockerfile": onBuild, "requirements.txt": "pandas==2.1.0\n"})
		warning, err := CheckDockerfileOnBuild(dir, load(dir))
		require.ErrorIs(t, err, ErrOnBuildBase)
		assert.Contains(t, err.Error(), "Run astro init")
		assert.Empty(t, warning)
	})
	t.Run("the refusal's lines keep the stage's name and flags, as init writes them", func(t *testing.T) {
		dir := v2Project(t, map[string]string{"Dockerfile": "FROM --platform=linux/amd64 quay.io/astronomer/astro-runtime:12.6.0 AS app\nRUN true\n"})
		_, err := CheckDockerfileOnBuild(dir, load(dir))
		require.ErrorIs(t, err, ErrOnBuildBase)
		assert.Contains(t, err.Error(), airflowrt.InstallDockerfileWith("--platform=linux/amd64 quay.io/astronomer/astro-runtime:12.6.0-base",
			airflowrt.InstallOptions{Alias: "app", ListsFrom: airflowrt.InstallContext}))
		assert.Contains(t, err.Error(), "astro package --context-dir")
	})
	t.Run("warned about when a list drifted", func(t *testing.T) {
		dir := v2Project(t, map[string]string{"Dockerfile": onBuild, "requirements.txt": "pandas==2.1.0\nrequests==2.32.0\n"})
		warning, err := CheckDockerfileOnBuild(dir, load(dir))
		require.NoError(t, err)
		assert.Contains(t, warning, "requirements.txt: requests==2.32.0 is not in [project] dependencies")
		assert.Contains(t, warning, "each build of it warns, and installs requirements.txt and packages.txt through that tag's ONBUILD steps")
	})
	t.Run("warned about when a gate blocks the move", func(t *testing.T) {
		dir := v2Project(t, map[string]string{"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0 AS deps\nRUN true\n" + onBuild})
		warning, err := CheckDockerfileOnBuild(dir, load(dir))
		require.NoError(t, err)
		assert.Contains(t, warning, "an earlier stage builds on quay.io/astronomer/astro-runtime:12.6.0")
	})
	t.Run("quiet on -base and with no Dockerfile", func(t *testing.T) {
		dir := v2Project(t, map[string]string{"Dockerfile": "FROM quay.io/astronomer/astro-runtime:12.6.0-base\nRUN true\n"})
		m := load(dir)
		warning, err := CheckDockerfileOnBuild(dir, m)
		require.NoError(t, err)
		assert.Empty(t, warning)
		m.Astro.Dockerfile = ""
		warning, err = CheckDockerfileOnBuild(dir, m)
		require.NoError(t, err)
		assert.Empty(t, warning)
	})
}
