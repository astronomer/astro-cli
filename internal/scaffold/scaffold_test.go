package scaffold

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/uv"
)

func TestRunFreshScaffold(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "flight-data")
	res, err := Run(dir, Options{})
	require.NoError(t, err)

	assert.Equal(t, dir, res.Dir)
	assert.Equal(t, "flight-data", res.Name)
	assert.Equal(t, DefaultAirflowVersion, res.AirflowVersion)
	assert.Empty(t, res.Skipped)

	for _, d := range []string{"dags", "include", "plugins", "tests"} {
		info, err := os.Stat(filepath.Join(dir, d))
		require.NoError(t, err, d)
		assert.True(t, info.IsDir(), d)
		if runtime.GOOS != windowsOS {
			assert.Equal(t, os.FileMode(0o755), info.Mode().Perm(), d)
		}
	}
	for _, f := range []string{"pyproject.toml", ".gitignore", "AGENTS.md"} {
		info, err := os.Stat(filepath.Join(dir, f))
		require.NoError(t, err, f)
		if runtime.GOOS != windowsOS {
			assert.Equal(t, os.FileMode(0o644), info.Mode().Perm(), f)
		}
	}

	// The scaffolded manifest must load and carry the derived values.
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "flight-data", m.Project.Name)
	assert.Equal(t, DefaultAirflowVersion, m.Astro.AirflowVersion)
	// [project.dependencies] must install the pinned Airflow, so init → start
	// works with no hand-edit. The default pin is partial, so the
	// requirement is a prefix match.
	assert.Equal(t, []string{"apache-airflow==3.1.*"}, m.Project.Dependencies)
	// A greenfield project declares no OS packages: the manifest carries no
	// packages key at all, not an empty list.
	assert.Nil(t, m.Astro.Packages)
	data, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(data), "packages")
}

// TestScaffoldedProjectLocksWithRealUv scaffolds a project and runs a real
// `uv lock` against it, proving init produces a manifest uv can actually
// resolve — the end-to-end gap an earlier fix closed. It needs uv and network, so it
// skips under -short or when uv is absent (as CI is).
func TestScaffoldedProjectLocksWithRealUv(t *testing.T) {
	if testing.Short() {
		t.Skip("real uv lock reaches the network")
	}
	if _, err := exec.LookPath("uv"); err != nil {
		t.Skip("uv not on PATH")
	}

	dir := filepath.Join(t.TempDir(), "locktest")
	_, err := Run(dir, Options{})
	require.NoError(t, err)

	client, err := uv.New(t.Context(), uv.Options{CacheDir: t.TempDir()})
	require.NoError(t, err)
	require.NoError(t, client.Lock(t.Context(), dir, uv.Stdio{}),
		"a freshly scaffolded project must uv lock cleanly")

	_, err = os.Stat(filepath.Join(dir, "uv.lock"))
	require.NoError(t, err, "uv lock must write a lockfile")
}

func TestRunHonorsNameAndAirflowVersion(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(dir, Options{Name: "etl", AirflowVersion: "3.0.2"})
	require.NoError(t, err)
	assert.Equal(t, "etl", res.Name)
	assert.Equal(t, "3.0.2", res.AirflowVersion)

	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "etl", m.Project.Name)
	assert.Equal(t, "3.0.2", m.Astro.AirflowVersion)
	// A full three-part pin becomes an exact requirement, not a prefix match.
	assert.Equal(t, []string{"apache-airflow==3.0.2"}, m.Project.Dependencies)
}

func TestRunRejectsInvalidNameAndVersion(t *testing.T) {
	_, err := Run(t.TempDir(), Options{Name: "-bad-"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project.name")

	_, err = Run(t.TempDir(), Options{AirflowVersion: "latest"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool.astro.airflow")
}

func TestDeriveName(t *testing.T) {
	cases := map[string]string{
		"flight-data":   "flight-data",
		"My Project":    "my-project",
		"data_pipeline": "data_pipeline",
		"v2.1":          "v2.1",
		"--weird--":     "weird",
		"...":           "astro-project",
		"héllo wörld":   "h-llo-w-rld",
	}
	for in, want := range cases {
		assert.Equal(t, want, deriveName(in), in)
	}
}

func TestRunRefusesAnAstroProject(t *testing.T) {
	dir := t.TempDir()
	m := "[project]\nname = 'orders'\n\n[tool.astro]\nairflow = '3.1'\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(m), 0o600))
	_, err := Run(dir, Options{})
	require.ErrorIs(t, err, ErrAlreadyAstroProject)
	assert.Contains(t, err.Error(), dir)
}

func TestRunAdoptsExistingManifest(t *testing.T) {
	dir := t.TempDir()
	// Comments and key order must survive: this is the user's own file, and
	// the port that follows is reviewed as a diff.
	existing := "# our project\n[project]\nname = 'orders'\nrequires-python = '>=3.11'\ndependencies = [\n    'requests==2.31.0',  # http\n]\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, "orders", res.Name)
	assert.Equal(t, DefaultAirflowVersion, res.AirflowVersion)
	assert.NotEmpty(t, res.Updated)

	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	got := string(out)
	assert.Contains(t, got, "# our project")
	assert.Contains(t, got, "# http")
	assert.Contains(t, got, "requests==2.31.0")
	assert.Contains(t, got, "requires-python = '>=3.11'")
	assert.Contains(t, got, airflowRequirement(DefaultAirflowVersion))
	m, err := manifest.Parse(out)
	require.NoError(t, err)
	assert.Equal(t, DefaultAirflowVersion, m.Astro.AirflowVersion)

	// The rest of the scaffold lands beside it.
	for _, f := range []string{"dags", "include", "plugins", "tests", ".gitignore", "AGENTS.md"} {
		_, statErr := os.Lstat(filepath.Join(dir, f))
		assert.NoError(t, statErr, f)
	}
}

func TestRunAdoptsThePinAlreadyInDependencies(t *testing.T) {
	dir := t.TempDir()
	existing := "[project]\nname = 'orders'\ndependencies = ['apache-airflow==2.9.3', 'requests']\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	// A project running 2.9 stays on 2.9: init reads the pin, never moves it.
	assert.Equal(t, "2.9.3", res.AirflowVersion)

	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(out), "apache-airflow"), "the pin was duplicated:\n%s", out)
}

// PEP 621 requires a version beside the name, and uv refuses to build the
// environment without one. This package's validation does not ask for it, so a
// manifest missing it would parse here and fail at the first `local start`.
func TestRunWritesAProjectVersionWhenAdopting(t *testing.T) {
	version := func(t *testing.T, manifest string) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(manifest), 0o600))
		_, err := Run(dir, Options{})
		require.NoError(t, err)
		out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
		require.NoError(t, err)
		return string(out)
	}

	// No [project] table at all: the one this run creates carries a version.
	assert.Contains(t, version(t, "[tool.ruff]\nline-length = 100\n"), "version = '0.1.0'")

	// A [project] table without one gains it.
	assert.Contains(t, version(t, "[project]\nname = 'a'\n"), "version = '0.1.0'")

	// A version already there is never rewritten.
	got := version(t, "[project]\nname = 'a'\nversion = '2.4.0'\n")
	assert.Contains(t, got, "version = '2.4.0'")
	assert.NotContains(t, got, "0.1.0")

	// A version a build backend supplies satisfies PEP 621, so leave it alone.
	got = version(t, "[project]\nname = 'a'\ndynamic = ['version']\n")
	assert.NotContains(t, got, "version = '0.1.0'")
	assert.Contains(t, got, "dynamic = ['version']")
}

func TestRunAdoptsAManifestWithNoProjectTable(t *testing.T) {
	dir := t.TempDir()
	// Poetry-era files carry no [project] table at all, so there is no name
	// for the manifest's own validation to accept.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[tool.ruff]\nline-length = 100\n"), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, deriveName(dir), res.Name)

	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(out), "line-length = 100")
	_, err = manifest.Parse(out)
	require.NoError(t, err)
}

// [project] and [tool.astro] are what make the directory an Astro project, so
// they lead the manifest instead of trailing every tool's own section
//.
func TestRunAdoptsPuttingTheAstroSectionsFirst(t *testing.T) {
	dir := t.TempDir()
	tools := "[tool.sqlfluff]\ndialect = 'snowflake'\n\n# AIR = airflow ruleset\n[tool.ruff.lint]\nselect = ['AIR']\n\n[tool.mypy]\nstrict = true\n"
	existing := "# Copyright ACME\n# SPDX-License-Identifier: Apache-2.0\n\n" + tools
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600))

	_, err := Run(dir, Options{Name: "orders"})
	require.NoError(t, err)

	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	got := string(out)
	// The license header keeps the top of the file; the two new sections come
	// below it, [project] first, and the rest of the file is untouched.
	assert.True(t, strings.HasPrefix(got, "# Copyright ACME\n# SPDX-License-Identifier: Apache-2.0\n\n[project]\n"), got)
	assert.True(t, strings.HasSuffix(got, tools), got)
	assert.Less(t, strings.Index(got, "[tool.astro]"), strings.Index(got, "[tool.sqlfluff]"), got)

	m, err := manifest.Parse(out)
	require.NoError(t, err)
	assert.Equal(t, "orders", m.Project.Name)
	assert.Equal(t, DefaultAirflowVersion, m.Astro.AirflowVersion)
}

// Most repos state the Airflow they run in a Dockerfile image tag, and most of
// those are on 2.x. init does not read the tag, so it must at least say that
// the pin it wrote is a default nobody chose.
func TestRunWarnsWhenThePinIsJustTheDefault(t *testing.T) {
	withDockerfile := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
			[]byte("FROM quay.io/astronomer/astro-runtime:9.1.0\n"), 0o600))
		return dir
	}
	defaultPinNote := func(notes []string) bool {
		return strings.Contains(strings.Join(notes, "\n"), "is the default, not this project's version")
	}

	res, err := Run(withDockerfile(t), Options{})
	require.NoError(t, err)
	assert.True(t, defaultPinNote(res.Notes), "no warning for a defaulted pin: %v", res.Notes)

	// Naming the version settles it, so there is nothing to warn about.
	res, err = Run(withDockerfile(t), Options{AirflowVersion: "2.9.3"})
	require.NoError(t, err)
	assert.False(t, defaultPinNote(res.Notes), "warned about a version the caller chose: %v", res.Notes)

	// An empty directory has no better answer sitting in it, so no nagging.
	res, err = Run(t.TempDir(), Options{})
	require.NoError(t, err)
	assert.False(t, defaultPinNote(res.Notes), "warned with nothing to read: %v", res.Notes)

	// A manifest that already pins Airflow settles it too.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"), []byte("FROM x\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"),
		[]byte("[project]\nname='a'\ndependencies=['apache-airflow==2.9.3']\n"), 0o600))
	res, err = Run(dir, Options{})
	require.NoError(t, err)
	assert.False(t, defaultPinNote(res.Notes), "warned about a pin read from the manifest: %v", res.Notes)
}

// PEP 621 forbids a static dependencies array beside a dynamic declaration,
// and uv refuses the manifest outright — which is the shape of every repo that
// keeps its dependencies in a requirements.txt.
func TestRunLeavesDynamicDependenciesAlone(t *testing.T) {
	dir := t.TempDir()
	existing := "[project]\nname = 'a'\ndynamic = ['version', 'dependencies']\n\n" +
		"[tool.setuptools.dynamic]\ndependencies = {file = ['requirements.txt']}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.NotContains(t, string(out), "apache-airflow", "a static dependency was added beside a dynamic one:\n%s", out)
	assert.Contains(t, string(out), "dynamic = ['version', 'dependencies']")
	// The pin still has to reach the project, so init says where to put it.
	assert.Contains(t, strings.Join(res.Notes, "\n"), "dependencies are dynamic")
}

// A range or a wildcard names an Airflow this cannot read a version out of, so
// the default lands instead — which for a 2.x project is a whole generation.
// Nothing else in such a repo need mention a version, so the manifest itself
// has to trigger the warning.
func TestRunWarnsWhenAPinnedProjectFallsBackToTheDefault(t *testing.T) {
	dir := t.TempDir()
	existing := "[project]\nname = 'a'\nversion = '0.1.0'\ndependencies = ['apache-airflow[celery]>=2.9,<2.10']\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(existing), 0o600))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, DefaultAirflowVersion, res.AirflowVersion)
	assert.Contains(t, strings.Join(res.Notes, "\n"), "is the default, not this project's version")

	// The project's own requirement is left as its author wrote it.
	out, err := os.ReadFile(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Contains(t, string(out), "apache-airflow[celery]>=2.9,<2.10")
	assert.Equal(t, 1, strings.Count(string(out), "apache-airflow"))
}

func TestRunAdoptsAV1ProjectAndListsTheLeftovers(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"Dockerfile":            "FROM quay.io/astronomer/astro-runtime:9\n",
		"requirements.txt":      "flask==2.0\n",
		"packages.txt":          "libpq-dev\n",
		"airflow_settings.yaml": "airflow:\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	require.NoError(t, os.Mkdir(filepath.Join(dir, ".astro"), 0o700))

	// init writes the manifest and names what it could not read, which is the
	// work the port picks up.
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	joined := strings.Join(res.Notes, "\n")
	for _, want := range []string{"Dockerfile", "requirements.txt", "packages.txt", "airflow_settings.yaml"} {
		assert.Contains(t, joined, want)
	}
}

func TestRunKeepsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	own := []byte("# mine\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitignore"), own, 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "dags"), 0o700))

	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Skipped, ".gitignore")
	assert.Contains(t, res.Skipped, "dags/")

	// The hand-written .gitignore is kept, but init heals it to cover .env so
	// local values are never committed. That is an edit to a file that was
	// already there, so it is reported as one — listing it as created too
	// would contradict the "already existed, kept" line beside it.
	assert.Contains(t, res.Updated, ".gitignore (added the .env rule)")
	got, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(got), "# mine\n"), "original content kept: %q", string(got))
	assert.Contains(t, string(got), "\n.env\n")
}

func TestRunSymlinksClaudeMdOnUnix(t *testing.T) {
	if runtime.GOOS == windowsOS {
		t.Skip("symlink layout is unix-only")
	}
	dir := t.TempDir()
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Created, "CLAUDE.md -> AGENTS.md")

	link := filepath.Join(dir, "CLAUDE.md")
	info, err := os.Lstat(link)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "CLAUDE.md must be a symlink")
	target, err := os.Readlink(link)
	require.NoError(t, err)
	assert.Equal(t, "AGENTS.md", target)

	// An existing CLAUDE.md (the user's own) is kept.
	dir2 := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "CLAUDE.md"), []byte("mine"), 0o600))
	res, err = Run(dir2, Options{})
	require.NoError(t, err)
	assert.Contains(t, res.Skipped, "CLAUDE.md")
}

func TestRunWindowsGetsAgentsMdAlone(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(dir, Options{GOOS: windowsOS})
	require.NoError(t, err)

	_, err = os.Lstat(filepath.Join(dir, "AGENTS.md"))
	require.NoError(t, err)
	_, err = os.Lstat(filepath.Join(dir, "CLAUDE.md"))
	require.ErrorIs(t, err, os.ErrNotExist)
	for _, entry := range append(res.Created, res.Skipped...) {
		assert.NotContains(t, entry, "CLAUDE.md")
	}
}

func TestRunCreatesMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", "b", "new-project")
	res, err := Run(dir, Options{})
	require.NoError(t, err)
	assert.Equal(t, "new-project", res.Name)
	_, err = os.Stat(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
}

func TestAgentsMdCarriesTheDevMapping(t *testing.T) {
	content := agentsContent()
	for _, m := range DevReplacements() {
		row := "| `astro dev " + m.Command + "` | `" + m.Replacement + "` |"
		assert.Contains(t, content, row)
	}
	assert.Contains(t, content, "pyproject.toml")
	assert.NotContains(t, content, DefaultAirflowVersion,
		"AGENTS.md must reference the manifest, not duplicate its values")
}
