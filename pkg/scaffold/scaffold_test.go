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
	// A tag that is not an Astro Runtime version at all. This used to be a
	// Dockerfile tagged 9.1.0, which now READS as Airflow 2 and so has nothing
	// left to warn about — see TestRunReadsTheAirflowVersionFromTheDockerfile.
	// What survives is the narrower case the warning is now for: a file stated
	// a version, and it was not one we could use.
	withDockerfile := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
			[]byte("FROM quay.io/astronomer/astro-runtime:latest\n"), 0o600))
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

func TestRunConvertsAV1ProjectAndListsOnlyWhatIsLeft(t *testing.T) {
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

	res, err := Run(dir, Options{})
	require.NoError(t, err)

	// The three files init now READS are carried into the manifest.
	m, err := manifest.Load(filepath.Join(dir, "pyproject.toml"))
	require.NoError(t, err)
	assert.Equal(t, "2", res.AirflowVersion, "the Dockerfile's runtime 9 is an Airflow 2 image")
	assert.Equal(t, "2", m.Astro.AirflowVersion)
	// The Airflow requirement leads and the carried pin follows it. A partial
	// pin becomes a prefix match, the same rule the greenfield path uses.
	assert.Equal(t, []string{"apache-airflow==2.*", "flask==2.0"}, m.Project.Dependencies)
	assert.Equal(t, []string{"libpq-dev"}, m.Astro.Packages)

	// And the hand-off list is now only what init did not read. This assertion
	// is the point of the change: naming a file it already carried would be
	// telling the user to do work that is done.
	joined := strings.Join(res.Notes, "\n")
	assert.Contains(t, joined, "airflow_settings.yaml")
	assert.NotContains(t, joined, "requirements.txt: move")
	assert.NotContains(t, joined, "packages.txt: move")
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

// A new project starts with a DAG, and an existing one keeps its own.
//
// Two questions that look like one. Skip-existing answers "is there a file
// called exampledag.py", which would drop an example into a repo full of real
// pipelines just because nothing there carried that name, and adoption is the
// common case for init in an existing repo. What decides it is whether the
// project has any DAGs at all.
func TestStarterDag(t *testing.T) {
	t.Run("a new project gets one", func(t *testing.T) {
		dir := t.TempDir()
		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		assert.Contains(t, res.Created, "dags/exampledag.py")
		assert.FileExists(t, filepath.Join(dir, "dags", "exampledag.py"))
	})

	t.Run("a project with DAGs keeps its own", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "my_pipeline.py"), []byte("# theirs\n"), 0o600))

		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		assert.NotContains(t, res.Created, "dags/exampledag.py")
		assert.NoFileExists(t, filepath.Join(dir, "dags", "exampledag.py"))
		// And theirs is untouched.
		body, err := os.ReadFile(filepath.Join(dir, "dags", "my_pipeline.py"))
		require.NoError(t, err)
		assert.Equal(t, "# theirs\n", string(body))
	})

	t.Run("an empty dags directory still counts as new", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o750))
		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		assert.Contains(t, res.Created, "dags/exampledag.py")
	})

	// Git cannot track an empty directory, so a repository that committed an
	// empty dags/ carries a placeholder in it. Counting that entry would deny
	// the example to the commonest shape of a project with no DAGs.
	t.Run("a placeholder is not a DAG", func(t *testing.T) {
		for _, name := range []string{".gitkeep", ".gitignore", ".DS_Store", "__pycache__"} {
			t.Run(name, func(t *testing.T) {
				dir := t.TempDir()
				require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o750))
				require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", name), nil, 0o600))

				res, err := Run(dir, Options{Name: "p"})
				require.NoError(t, err)
				assert.Contains(t, res.Created, "dags/exampledag.py")
			})
		}
	})

	t.Run("an edited starter DAG is not replaced on a rerun", func(t *testing.T) {
		// Belt and braces twice over: adopt refuses a manifest carrying
		// [tool.astro] before reaching the files, so a rerun cannot get here at
		// all, and the file makes dags/ hold a DAG, so the example is never
		// planned in the first place. Neither of those is the skip-existing
		// check, which this file can never reach — it is only offered when dags/
		// holds no DAG, and a dags/ holding exampledag.py holds one.
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "dags"), 0o750))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dags", "exampledag.py"), []byte("# edited\n"), 0o600))

		_, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		body, err := os.ReadFile(filepath.Join(dir, "dags", "exampledag.py"))
		require.NoError(t, err)
		assert.Equal(t, "# edited\n", string(body))
	})
}

// The one property the starter DAG must hold: it imports nothing the scaffold
// does not install. [project.dependencies] names apache-airflow alone, so an
// unresolvable import is a DAG that fails to load on the first
// `astro local start` — a worse first run than no example at all.
//
// Asserted as an allowlist. A denylist of libraries passes everything nobody
// thought to ban, which is every library except the few that came to mind: the
// three names this used to check let numpy, boto3, httpx and
// `from pandas import DataFrame` straight through.
func TestStarterDagImportsOnlyWhatTheScaffoldInstalls(t *testing.T) {
	allowed := map[string]bool{manifestKeyAirflow: true, "datetime": true}

	var checked int
	for _, line := range strings.Split(exampleDag, "\n") {
		fields := strings.Fields(line)
		// Matched on shape rather than on the first word alone, because the
		// module docstring is prose and "from the function call rather than
		// wired up by hand." is not an import of a package called "the".
		var isImport bool
		switch {
		case len(fields) < 2:
		case fields[0] == "import":
			isImport = len(fields) == 2 || (len(fields) == 4 && fields[2] == "as")
		case fields[0] == "from":
			isImport = len(fields) >= 4 && fields[2] == "import"
		}
		if !isImport {
			continue
		}
		root, _, _ := strings.Cut(fields[1], ".")
		checked++
		assert.Truef(t, allowed[root],
			"the starter DAG imports %q, which the scaffold does not install; "+
				"name it in [project.dependencies] or widen this allowlist deliberately", root)
	}
	// Or a rename turns the loop above into a test that asserts nothing.
	require.GreaterOrEqual(t, checked, 2, "found no imports to check in the starter DAG")
}

// The rule above is settled by reading the file. That the file is Python at all
// is not, and a typo in it would otherwise ship green and surface as a parse
// error on somebody's first start.
//
// Skipped rather than failed where there is no interpreter: this is a Go
// package, and a machine without python3 is not a reason to fail its build.
func TestStarterDagIsValidPython(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not on PATH")
	}
	path := filepath.Join(t.TempDir(), "exampledag.py")
	require.NoError(t, os.WriteFile(path, []byte(exampleDag), 0o600))

	out, err := exec.Command(python, "-c",
		"import ast,sys; ast.parse(open(sys.argv[1]).read())", path).CombinedOutput()
	require.NoError(t, err, "the starter DAG is not valid Python:\n%s", out)
}

// The example imports airflow.sdk, which is the Airflow 3 Task SDK. Airflow 2
// spells the same decorators airflow.decorators, so on a project pinning 2 the
// example would fail to import on the first start — the failure the whole
// feature is built to avoid. The pin reaches planFiles through
// cs.AirflowVersion, and pickAirflowVersion sources it from a flag, the
// manifest, a Dockerfile tag or requirements.txt, so 2 is an ordinary answer
// rather than an exotic one.
func TestStarterDagNeedsAirflow3(t *testing.T) {
	for _, tc := range []struct {
		version string
		want    bool
	}{
		{"3.1", true},
		{"3", true},
		{"3.0.2", true},
		{"4", true},
		{"2", false},
		{"2.10.5", false},
	} {
		t.Run(tc.version, func(t *testing.T) {
			dir := t.TempDir()
			res, err := Run(dir, Options{Name: "p", AirflowVersion: tc.version})
			require.NoError(t, err)

			if tc.want {
				assert.Contains(t, res.Created, "dags/exampledag.py")
				return
			}
			assert.NotContains(t, res.Created, "dags/exampledag.py")
			assert.NoFileExists(t, filepath.Join(dir, "dags", "exampledag.py"),
				"an Airflow 2 project got a DAG that cannot import on Airflow 2")
		})
	}

	// The flag is the least likely of the four sources. Adopting a v1 project
	// whose Dockerfile names an Airflow 2 runtime is the ordinary way a 2 pin
	// arrives, and it reaches the same decision by a different road.
	t.Run("adopted from an Airflow 2 Dockerfile", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Dockerfile"),
			[]byte("FROM quay.io/astronomer/astro-runtime:9.6.0\n"), 0o600))

		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		require.Equal(t, "2", res.AirflowVersion, "the Dockerfile no longer pins Airflow 2")
		assert.NotContains(t, res.Created, "dags/exampledag.py")
		assert.NoFileExists(t, filepath.Join(dir, "dags", "exampledag.py"))
	})
}

// The starter DAG is the first file written to a nested path, so `dags` being
// something other than a plain directory stopped being someone else's problem.
// None of these shapes may fail the run: init scaffolded them all before the
// example existed, and refusing to init over one would be a regression.
func TestStarterDagLeavesAnOddDagsEntryAlone(t *testing.T) {
	t.Run("a regular file named dags", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dags"), []byte("not a directory\n"), 0o600))

		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err, "a file named dags used to scaffold fine and must still")
		assert.NotContains(t, res.Created, "dags/exampledag.py")
		body, err := os.ReadFile(filepath.Join(dir, "dags"))
		require.NoError(t, err)
		assert.Equal(t, "not a directory\n", string(body))
	})

	if runtime.GOOS == windowsOS {
		return // symlinks need a privilege the runner may not hold
	}

	t.Run("a dangling dags symlink", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.Symlink(filepath.Join(dir, "nowhere"), filepath.Join(dir, "dags")))

		// os.ReadDir follows the link, so this read as an empty dags/ and the
		// write then failed part way through the run.
		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		assert.NotContains(t, res.Created, "dags/exampledag.py")
		assert.FileExists(t, filepath.Join(dir, "pyproject.toml"), "the run died before the manifest")
	})

	t.Run("a dags symlink pointing outside the project", func(t *testing.T) {
		dir, outside := t.TempDir(), t.TempDir()
		require.NoError(t, os.Symlink(outside, filepath.Join(dir, "dags")))

		res, err := Run(dir, Options{Name: "p"})
		require.NoError(t, err)
		assert.NotContains(t, res.Created, "dags/exampledag.py")
		// Change.resolve undertakes that a change set never writes outside the
		// project. It cleans the declared path but does not resolve symlinks, so
		// the undertaking holds only while nothing writes THROUGH one.
		assert.NoFileExists(t, filepath.Join(outside, "exampledag.py"),
			"the scaffold wrote outside the directory the preview showed")
	})
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
