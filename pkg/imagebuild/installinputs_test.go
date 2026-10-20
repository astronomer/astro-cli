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

	"github.com/astronomer/astro-cli/pkg/manifest"
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
	out := runHelper(t, dir, helperRun{runtimeConstraints: runtimeConstraints})
	return out.requirements, out.packages, out.constraints
}

// helperRun is how runHelper runs the helper: the runtime constraints file it
// reads ("" for none), and environment variables besides the test's own.
type helperRun struct {
	runtimeConstraints string
	env                []string
}

// helperOutput is what one run of the helper wrote, and its warnings.
type helperOutput struct {
	requirements, packages, constraints, stderr string
}

// runHelper runs the install helper over dir as the install stage does,
// skipping the test when the host has no Python to run it on.
func runHelper(t *testing.T, dir string, r helperRun) helperOutput {
	t.Helper()
	py := hostPython(t)
	if py == "" {
		t.Skip("no Python 3.8+ with tomllib or tomli on PATH to run the install helper on")
	}
	if r.runtimeConstraints == "" {
		r.runtimeConstraints = filepath.Join(t.TempDir(), "no-runtime-constraints")
	}
	out := t.TempDir()
	var stderr bytes.Buffer
	cmd := exec.Command(py, filepath.Join(dir, filepath.FromSlash(InstallHelperPath)), "--project", dir, "--out", out, "--runtime-constraints", r.runtimeConstraints)
	cmd.Env = append(os.Environ(), r.env...)
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err, name)
		return string(data)
	}
	return helperOutput{read(requirementsName), read(packagesName), read(lockConstraintsName), stderr.String()}
}

// withoutPackaging runs the helper as on a Python whose packaging it cannot
// use, so it falls back to its own marker evaluator and specifier comparison.
var withoutPackaging = []string{"ASTRO_INSTALL_PROJECT_NO_PACKAGING=1"}

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
// resolving them: the project itself, recording each dependency's name and
// specifier as uv does, then packages.
func lockProject(deps []string, packages string) (pyproject, lock string) {
	quoted := make([]string, len(deps))
	requiresDist := make([]string, len(deps))
	for i, d := range deps {
		quoted[i] = "'" + d + "'"
		name, spec := d, ""
		if at := strings.IndexAny(d, "<>=!~"); at >= 0 {
			name, spec = d[:at], d[at:]
		}
		requiresDist[i] = `{ name = "` + name + `"`
		if spec != "" {
			requiresDist[i] += `, specifier = "` + spec + `"`
		}
		requiresDist[i] += " }"
	}
	pyproject = "[project]\nname = 'p'\ndependencies = [" + strings.Join(quoted, ", ") + "]\n"
	lock = "version = 1\nrequires-python = \">=3.8\"\n\n" +
		"[[package]]\nname = \"p\"\nversion = \"0.1.0\"\nsource = { virtual = \".\" }\n\n" +
		"[package.metadata]\nrequires-dist = [" + strings.Join(requiresDist, ", ") + "]\n\n" + packages
	return pyproject, lock
}

// The registry entry of a package in a uv.lock.
func lockEntry(name, version string) string {
	return "[[package]]\nname = \"" + name + "\"\nversion = \"" + version + "\"\nsource = { registry = \"https://pypi.org/simple\" }\n\n"
}

// With a uv.lock, every package the lock resolved from an index is held at
// its version, through a constraints file requirements.txt names with -c.
// The same with packaging and with the helper's own marker evaluator, which a
// runtime whose packaging cannot read a resolution marker falls back to.
func TestInstallHelperPinsTheLocksVersions(t *testing.T) {
	pyproject, lock := lockProject([]string{"apache-airflow==2.10.*", "pandas>=2,<3", "six"}, lockEntry("pandas", "2.2.3")+
		lockEntry("Python_Dateutil", "2.9.0")+`
[[package]]
name = "six"
version = "1.16.0"
source = { git = "https://github.com/benjaminp/six?rev=1.16.0#65486e4383f9f411da95937451205d3c7b61b9e1" }

[[package]]
name = "apache-airflow"
version = "2.10.4+astro.1"
source = { registry = "https://pip.astronomer.io/v2/" }

[[package]]
name = "sqlalchemy"
version = "1.4.54"
source = { registry = "https://pypi.org/simple" }

[[package]]
name = "numpy"
version = "2.2.6"
source = { registry = "https://pypi.org/simple" }
resolution-markers = [
    "python_full_version < '3.0' and platform_machine != 'not-a-machine'",
    "python_full_version < '3.0' and platform_machine == 'arm64' and sys_platform == 'darwin'",
]

[[package]]
name = "numpy"
version = "2.4.6"
source = { registry = "https://pypi.org/simple" }
resolution-markers = [
    "python_full_version >= '3.0' and platform_machine != 'not-a-machine'",
    "(python_full_version >= '3.0' and platform_machine == 'arm64' and sys_platform == 'darwin') or python_full_version == '2.7.*'",
]
`)
	dir := helperProject(t, pyproject, lock)
	runtime := filepath.Join(t.TempDir(), "pip-constraints.txt")
	require.NoError(t, os.WriteFile(runtime, []byte("apache-airflow==2.10.4+astro.1\nSQLAlchemy==1.4.54\n"), 0o600))

	for name, env := range map[string][]string{"packaging": nil, "own evaluator": withoutPackaging} {
		t.Run(name, func(t *testing.T) {
			out := runHelper(t, dir, helperRun{runtimeConstraints: runtime, env: env})
			assert.Equal(t, "-c "+lockConstraintsName+"\npandas>=2,<3\nsix\n", out.requirements)
			// The fork this interpreter is on, by the entries' markers; nothing
			// from another source, nothing of Airflow's, nothing the runtime pins
			// itself.
			assert.Equal(t, "# Generated from uv.lock by .astro/install-project.py.\nnumpy==2.4.6\npandas==2.2.3\npython-dateutil==2.9.0\n", out.constraints)
			assert.Empty(t, out.stderr)
		})
	}
}

// The runtime's constraints file may name another; what that one pins, the
// runtime decides too.
func TestInstallHelperFollowsTheRuntimesNestedConstraints(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas"}, lockEntry("pandas", "2.2.3")+lockEntry("sqlalchemy", "2.1.5"))
	dir := helperProject(t, pyproject, lock)
	runtimeDir := t.TempDir()
	writeTree(t, runtimeDir, map[string]string{
		"pip-constraints.txt":  "# the runtime's\n-c more/constraints.txt\n",
		"more/constraints.txt": "SQLAlchemy==2.0.50\n-r pip-constraints.txt\n",
	})
	out := runHelper(t, dir, helperRun{runtimeConstraints: filepath.Join(runtimeDir, "pip-constraints.txt")})
	assert.Equal(t, "# Generated from uv.lock by .astro/install-project.py.\npandas==2.2.3\n", out.constraints)
}

// A lock that no longer resolves pyproject.toml's dependencies as they stand
// would pin versions the install refuses, so it is set aside with a warning
// and the install goes unpinned, as with no lock. Never a failed build.
func TestInstallHelperIgnoresALockThatIsOutOfDate(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas>=2,<3"}, lockEntry("pandas", "2.2.3"))
	for name, tc := range map[string]struct {
		from, to string
	}{
		"a dependency added":        {"'pandas>=2,<3'", "'pandas>=2,<3', 'polars'"},
		"a specifier changed":       {"'pandas>=2,<3'", "'pandas>=3'"},
		"the same specifier, moved": {"", ""},
		"an extra added":            {"'pandas>=2,<3'", "'pandas[excel]>=2,<3'"},
		"a lock it cannot read":     {"", ""},
	} {
		for mode, env := range map[string][]string{"packaging": nil, "own evaluator": withoutPackaging} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				p, l := strings.Replace(pyproject, tc.from, tc.to, 1), lock
				switch name {
				case "the same specifier, moved":
					p = strings.Replace(pyproject, "'pandas>=2,<3'", "'pandas<3,>=2'", 1)
				case "a lock it cannot read":
					l = "this is not = = toml"
				}
				out := runHelper(t, helperProject(t, p, l), helperRun{env: env})
				if name == "the same specifier, moved" {
					assert.Contains(t, out.constraints, "pandas==2.2.3", "the same requirement, written another way, is not out of date")
					assert.Empty(t, out.stderr)
					return
				}
				assert.Empty(t, out.constraints)
				assert.NotContains(t, out.requirements, "-c ")
				assert.Contains(t, out.stderr, "installing pyproject.toml's dependencies unpinned")
			})
		}
	}
}

// A dependency an index of its own serves is installed from the file uv.lock
// recorded, by URL with its hash, so the install never asks PyPI for a package
// of that name; without a lock it is that index's page for the package, and
// only for a package the image installs (Airflow comes with the image).
func TestInstallHelperIndexSources(t *testing.T) {
	const sources = "\n[[tool.uv.index]]\nname = 'acme'\nurl = 'https://pypi.acme.example/simple/'\nexplicit = true\n" +
		"[[tool.uv.index]]\nname = 'astronomer'\nurl = 'https://pip.astronomer.io/v2/'\nexplicit = true\n" +
		"[tool.uv.sources]\nprivate-lib = { index = 'acme' }\napache-airflow = { index = 'astronomer' }\n"
	pyproject, lock := lockProject([]string{"apache-airflow==3.3.*", "private-lib", "pandas"}, lockEntry("pandas", "2.2.3")+`
[[package]]
name = "private-lib"
version = "1.2.0"
source = { registry = "https://pypi.acme.example/simple/" }
sdist = { url = "https://pypi.acme.example/files/private_lib-1.2.0.tar.gz", hash = "sha256:aaaa" }
wheels = [
    { url = "https://pypi.acme.example/files/private_lib-1.2.0-cp30-cp30-weirdos_1_0.whl", hash = "sha256:bbbb" },
    { url = "https://pypi.acme.example/files/private_lib-1.2.0-py3-none-any.whl", hash = "sha256:cccc" },
]
`)
	pyproject += sources

	t.Run("no lock", func(t *testing.T) {
		out := runHelper(t, helperProject(t, pyproject, ""), helperRun{})
		assert.Equal(t, "--find-links https://pypi.acme.example/simple/private-lib/\nprivate-lib\npandas\n", out.requirements)
	})
	t.Run("with a lock", func(t *testing.T) {
		out := runHelper(t, helperProject(t, pyproject, lock), helperRun{})
		assert.Equal(t, "-c "+lockConstraintsName+"\n"+
			"private-lib @ https://pypi.acme.example/files/private_lib-1.2.0-py3-none-any.whl#sha256=cccc\npandas\n", out.requirements)
		assert.NotContains(t, out.constraints, "private-lib", "its file pins it")
	})
	t.Run("with a lock, without packaging to pick a wheel", func(t *testing.T) {
		out := runHelper(t, helperProject(t, pyproject, lock), helperRun{env: withoutPackaging})
		assert.Contains(t, out.requirements, "private-lib @ https://pypi.acme.example/files/private_lib-1.2.0.tar.gz#sha256=aaaa\n")
	})
}

// The fallback evaluator reads the resolution markers uv writes, and refuses
// what it cannot read rather than guessing.
func TestInstallHelperMarkerEvaluator(t *testing.T) {
	py := hostPython(t)
	if py == "" {
		t.Skip("no Python 3.8+ with tomllib or tomli on PATH to run the install helper on")
	}
	dir := helperProject(t, "[project]\n", "")
	const script = `
import importlib.util, sys
spec = importlib.util.spec_from_file_location("h", sys.argv[1])
h = importlib.util.module_from_spec(spec)
spec.loader.exec_module(h)
env = {"python_full_version": "3.11.9", "python_version": "3.11", "sys_platform": "linux", "platform_machine": "x86_64"}
for line in sys.stdin.read().splitlines():
    try:
        print(h.evaluate_marker(line, env))
    except ValueError:
        print("refused")
`
	cases := []struct{ marker, want string }{
		{"python_full_version < '3.11' and sys_platform == 'linux'", "False"},
		{"python_full_version >= '3.11' and sys_platform == 'linux'", "True"},
		{"(python_full_version == '3.13.*' and platform_machine == 'arm64' and sys_platform == 'darwin') or (python_full_version == '3.11.*' and sys_platform == 'linux')", "True"},
		{"python_version != '3.11' or platform_machine == 'x86_64'", "True"},
		{"'linux' in sys_platform and python_version < '3.12'", "True"},
		{"sys_platform not in 'win32 cygwin'", "True"},
		{"extra == 'test'", "refused"},
		{"python_full_version >=", "refused"},
	}
	var input strings.Builder
	for _, c := range cases {
		input.WriteString(c.marker + "\n")
	}
	cmd := exec.Command(py, "-c", script, filepath.Join(dir, filepath.FromSlash(InstallHelperPath)))
	cmd.Stdin = strings.NewReader(input.String())
	got, err := cmd.Output()
	require.NoError(t, err)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	require.Len(t, lines, len(cases))
	for i, c := range cases {
		assert.Equal(t, c.want, lines[i], c.marker)
	}
}

// A plain `docker build .` of a project's own Dockerfile has no find-links file
// (only a CLI build of a request that names no project writes one), so the
// helper derives the pages from pyproject.toml itself: a [tool.uv.sources]
// entry naming a [[tool.uv.index]], for a package the image installs, as
// manifest.UV.IndexPagesFor does. A CLI build of the same project writes no
// file either, so both read the same pages.
func TestInstallHelperDerivesIndexPagesWithoutTheFile(t *testing.T) {
	const pyproject = "[project]\nname = 'p'\ndependencies = ['apache-airflow==3.3.*', 'Private_Lib>=1', 'pandas']\n" +
		"[[tool.uv.index]]\nname = 'acme'\nurl = 'https://pypi.acme.example/simple'\nexplicit = true\n" +
		"[[tool.uv.index]]\nname = 'astronomer'\nurl = 'https://pip.astronomer.io/v2/'\nexplicit = true\n" +
		"[tool.uv.sources]\nPrivate_Lib = { index = 'acme' }\napache-airflow = { index = 'astronomer' }\n"
	dir := helperProject(t, pyproject, "")
	require.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(installFindLinksPath)))
	out := runHelper(t, dir, helperRun{})
	assert.Equal(t, "--find-links https://pypi.acme.example/simple/Private_Lib/\nPrivate_Lib>=1\npandas\n", out.requirements)

	// The same page manifest.UV.IndexPagesFor gives the CLI.
	uv := manifest.UV{Sources: map[string]string{"Private_Lib": "https://pypi.acme.example/simple", "apache-airflow": "https://pip.astronomer.io/v2/"}}
	assert.Equal(t, []string{"https://pypi.acme.example/simple/Private_Lib/"}, uv.IndexPagesFor(runtimeDeps([]string{"apache-airflow==3.3.*", "Private_Lib>=1", "pandas"})))

	// And a CLI build of that project writes no find-links file of its own.
	project := t.TempDir()
	writeTree(t, project, map[string]string{"pyproject.toml": pyproject})
	inputs, err := installInputs(Request{ManifestDir: project, FindLinks: []string{"https://pypi.acme.example/simple/Private_Lib/"}})
	require.NoError(t, err)
	assert.NotContains(t, inputs, installFindLinksPath)
}
