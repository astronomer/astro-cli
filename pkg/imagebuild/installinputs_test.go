package imagebuild

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// hostPython is a Python that can run the install helper (3.11+, or one with
// tomli), or "" when the machine has none. The helper runs inside the build on
// the runtime image's Python; here it runs on the host's, so its output can be
// checked without a container engine. CI's runners all ship one.
func hostPython(t *testing.T) string {
	t.Helper()
	// The plain names first, then the versioned ones a machine whose python3
	// is too old to read TOML (macOS's own 3.9) has beside it.
	for _, name := range []string{"python3", "python", "python3.14", "python3.13", "python3.12", "python3.11"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if exec.Command(bin, "-c", "import sys; assert sys.version_info >= (3, 8)\ntry:\n import tomllib\nexcept ImportError:\n import tomli").Run() == nil {
			return bin
		}
	}
	return ""
}

// installLists runs the install helper over the build context in dir, as the
// generated Dockerfile's install stage does, and returns what it wrote:
// requirements.txt, packages.txt and the lock's constraints. It skips the
// test when the host has no Python to run it on.
func installLists(t *testing.T, dir string) (requirements, packages, constraints string) {
	t.Helper()
	return installListsWith(t, dir, filepath.Join(t.TempDir(), "no-runtime-constraints"))
}

// installListsWith is installLists with the runtime image's constraints file
// at runtimeConstraints.
func installListsWith(t *testing.T, dir, runtimeConstraints string) (requirements, packages, constraints string) {
	t.Helper()
	py := hostPython(t)
	if py == "" {
		t.Skip("no Python 3.8+ with tomllib or tomli on PATH to run the install helper on")
	}
	out := t.TempDir()
	var stderr bytes.Buffer
	cmd := exec.Command(py, filepath.Join(dir, filepath.FromSlash(InstallHelperPath)), "--project", dir, "--out", out, "--runtime-constraints", runtimeConstraints)
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err, name)
		return string(data)
	}
	return read(requirementsName), read(packagesName), read(lockConstraintsName)
}

// helperProject is a build context holding the install helper and the given
// pyproject.toml and, when lock is not "", uv.lock.
func helperProject(t *testing.T, pyproject, lock string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{"pyproject.toml": pyproject, InstallHelperPath: string(installHelper)}
	if lock != "" {
		files["uv.lock"] = lock
	}
	writeTree(t, dir, files)
	return dir
}

// The helper turns [project] dependencies and [tool.uv.sources] into the
// requirements the runtime's install reads, as the manifest reads them: no
// Airflow, a git or URL source as a direct reference, an index source as a
// --find-links page ahead of everything.
func TestInstallHelperRequirements(t *testing.T) {
	for name, tc := range map[string]struct {
		pyproject, want string
	}{
		"plain dependencies": {
			pyproject: "[project]\ndependencies = ['pandas==2.2.0', 'requests']\n",
			want:      "pandas==2.2.0\nrequests\n",
		},
		"no dependencies": {
			pyproject: "[project]\nname = 'p'\n",
			want:      "\n",
		},
		"airflow and a bare task sdk are dropped": {
			pyproject: "[project]\ndependencies = ['apache-airflow==3.1.*', 'Apache_Airflow-Core', 'apache-airflow-task-sdk', 'pandas']\n",
			want:      "pandas\n",
		},
		"a task sdk with a version stays": {
			pyproject: "[project]\ndependencies = ['apache-airflow==3.1.*', 'apache-airflow-task-sdk>=1.1']\n",
			want:      "apache-airflow-task-sdk>=1.1\n",
		},
		"a git source becomes a direct reference": {
			pyproject: "[project]\ndependencies = ['blueprint[extra]>=1 ; python_version >= \"3.10\"']\n" +
				"[tool.uv.sources]\nblueprint = { git = 'https://github.com/astronomer/blueprint', rev = '6f06313', subdirectory = 'pkg' }\n",
			want: "blueprint[extra] @ git+https://github.com/astronomer/blueprint@6f06313#subdirectory=pkg ; python_version >= \"3.10\"\n",
		},
		"a url source becomes a direct reference": {
			pyproject: "[project]\ndependencies = ['six']\n[tool.uv.sources]\nsix = { url = 'https://files.example/six-1.16.0-py2.py3-none-any.whl' }\n",
			want:      "six @ https://files.example/six-1.16.0-py2.py3-none-any.whl\n",
		},
		"a source limited by a marker, a path and a list are left alone": {
			pyproject: "[project]\ndependencies = ['a', 'b', 'c']\n[tool.uv.sources]\n" +
				"a = { git = 'https://example.com/a', marker = 'sys_platform == \"linux\"' }\nb = { path = '../b' }\n" +
				"c = [{ git = 'https://example.com/c', marker = 'sys_platform == \"linux\"' }]\n",
			want: "a\nb\nc\n",
		},
		"an index source is a find-links page": {
			pyproject: "[project]\ndependencies = ['private-lib', 'pandas']\n" +
				"[[tool.uv.index]]\nname = 'acme'\nurl = 'https://pypi.acme.example/simple/'\nexplicit = true\n" +
				"[tool.uv.sources]\nprivate-lib = { index = 'acme' }\n",
			want: "--find-links https://pypi.acme.example/simple/private-lib/\nprivate-lib\npandas\n",
		},
	} {
		t.Run(name, func(t *testing.T) {
			reqs, _, constraints := installLists(t, helperProject(t, tc.pyproject, ""))
			assert.Equal(t, tc.want, reqs)
			assert.Empty(t, constraints, "no lock, no constraints")
		})
	}
}

// The runtime image ships no git, so a git direct reference brings git in
// through packages.txt, which the image installs before requirements.txt.
func TestInstallHelperPackages(t *testing.T) {
	for name, tc := range map[string]struct {
		deps, packages, want string
	}{
		"packages as listed":           {deps: "'pandas'", packages: "'libpq-dev', 'build-essential'", want: "libpq-dev\nbuild-essential\n"},
		"none is an empty file":        {deps: "'pandas'", packages: "", want: ""},
		"a git reference adds git":     {deps: "'x @ git+https://example.com/x.git'", packages: "'libpq-dev'", want: "libpq-dev\ngit\n"},
		"git already listed":           {deps: "'x @ git+https://example.com/x.git'", packages: "'git'", want: "git\n"},
		"git already pinned":           {deps: "'x @ git+https://example.com/x.git'", packages: "'git=1:2.39.5-0+deb12u2'", want: "git=1:2.39.5-0+deb12u2\n"},
		"the scheme in capitals":       {deps: "'x @ GIT+https://example.com/x.git'", want: "git\n"},
		"a plain url source adds none": {deps: "'x @ https://example.com/x-1.0.tar.gz'", want: ""},
		"a version pin adds none":      {deps: "'pandas>=2'", want: ""},
	} {
		t.Run(name, func(t *testing.T) {
			_, pkgs, _ := installLists(t, helperProject(t, "[project]\ndependencies = ["+tc.deps+"]\n[tool.astro]\npackages = ["+tc.packages+"]\n", ""))
			assert.Equal(t, tc.want, pkgs)
		})
	}
	t.Run("a git source adds git", func(t *testing.T) {
		_, pkgs, _ := installLists(t, helperProject(t, "[project]\ndependencies = ['x']\n[tool.uv.sources]\nx = { git = 'https://example.com/x' }\n", ""))
		assert.Equal(t, "git\n", pkgs)
	})
}

// lockProject is a pyproject.toml whose dependencies are deps, and a uv.lock
// resolving them: the project itself, then packages.
func lockProject(deps []string, packages string) (pyproject, lock string) {
	quoted := make([]string, len(deps))
	requiresDist := make([]string, len(deps))
	for i, d := range deps {
		quoted[i] = "'" + d + "'"
		requiresDist[i] = `{ name = "` + strings.Fields(d)[0] + `" }`
	}
	pyproject = "[project]\nname = 'p'\ndependencies = [" + strings.Join(quoted, ", ") + "]\n"
	lock = "version = 1\nrequires-python = \">=3.8\"\n\n" +
		"[[package]]\nname = \"p\"\nversion = \"0.1.0\"\nsource = { virtual = \".\" }\n\n" +
		"[package.metadata]\nrequires-dist = [" + strings.Join(requiresDist, ", ") + "]\n\n" + packages
	return pyproject, lock
}

// With a uv.lock, every package the lock resolved from an index is held at
// its version, through a constraints file requirements.txt names with -c.
func TestInstallHelperPinsTheLocksVersions(t *testing.T) {
	pyproject, lock := lockProject([]string{"apache-airflow", "pandas", "six"}, `
[[package]]
name = "pandas"
version = "2.2.3"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "Python_Dateutil"
version = "2.9.0"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "six"
version = "1.16.0"
source = { git = "https://github.com/benjaminp/six?rev=1.16.0#65486e4383f9f411da95937451205d3c7b61b9e1" }

[[package]]
name = "apache-airflow"
version = "3.3.2+astro.2"
source = { registry = "https://pip.astronomer.io/v2/" }

[[package]]
name = "apache-airflow-core"
version = "3.3.2+astro.2"
source = { registry = "https://pip.astronomer.io/v2/" }

[[package]]
name = "sqlalchemy"
version = "2.1.5"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "numpy"
version = "2.2.6"
source = { registry = "https://pypi.org/simple" }
resolution-markers = ["python_full_version < '3.0'"]

[[package]]
name = "numpy"
version = "2.4.6"
source = { registry = "https://pypi.org/simple" }
resolution-markers = ["python_full_version >= '3.0'"]
`)
	dir := helperProject(t, pyproject, lock)
	runtime := filepath.Join(t.TempDir(), "pip-constraints.txt")
	require.NoError(t, os.WriteFile(runtime, []byte("apache-airflow==3.3.2+astro.2\nSQLAlchemy==2.0.50\n"), 0o600))

	reqs, _, constraints := installListsWith(t, dir, runtime)
	assert.Equal(t, "-c "+lockConstraintsName+"\npandas\nsix\n", reqs)
	// The fork this interpreter is on, by the entries' markers; nothing from
	// another source, nothing of Airflow's, nothing the runtime pins itself.
	assert.Equal(t, "# Generated from uv.lock by .astro/install-project.py.\nnumpy==2.4.6\npandas==2.2.3\npython-dateutil==2.9.0\n", constraints)
}

// A lock that does not resolve a dependency pyproject.toml now lists is out of
// date, and pinning from it could hold a package to a version the new
// dependency cannot use, so the install goes unpinned, as with no lock.
func TestInstallHelperIgnoresALockThatIsOutOfDate(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas"}, "[[package]]\nname = \"pandas\"\nversion = \"2.2.3\"\nsource = { registry = \"https://pypi.org/simple\" }\n")
	pyproject = strings.Replace(pyproject, "'pandas'", "'pandas', 'polars'", 1)
	reqs, _, constraints := installLists(t, helperProject(t, pyproject, lock))
	assert.Equal(t, "pandas\npolars\n", reqs)
	assert.Empty(t, constraints)
}
