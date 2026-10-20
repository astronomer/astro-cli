package imagebuild

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/localrt/rt"
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
	out := runHelper(t, dir, helperRun{})
	return out.requirements, out.packages, out.constraints
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
		// An empty one, as a runtime that pins nothing would have. A missing
		// one is a runtime before 11.0.0, which reads the host's installed
		// packages as the base's (helperRun.env can name them).
		r.runtimeConstraints = filepath.Join(t.TempDir(), "pip-constraints.txt")
		require.NoError(t, os.WriteFile(r.runtimeConstraints, nil, 0o600))
	}
	out := t.TempDir()
	var stderr bytes.Buffer
	// A project's own build reads its inputs from the project; a generated
	// build's context has them in generatedInputsDir, as its stage copies them.
	project, helper := dir, filepath.Join(dir, filepath.FromSlash(InstallHelperPath))
	if inputs := filepath.Join(dir, filepath.FromSlash(generatedInputsDir)); dirExists(inputs) {
		project, helper = inputs, filepath.Join(inputs, filepath.Base(InstallHelperPath))
	}
	cmd := exec.Command(py, helper, "--project", project, "--out", out, "--runtime-constraints", r.runtimeConstraints)
	// The host's own packages are not a runtime image's: none, unless the test
	// names them.
	cmd.Env = append(append(os.Environ(), "ASTRO_INSTALL_PROJECT_BASE_INSTALLED="), r.env...)
	cmd.Stderr = &stderr
	require.NoError(t, cmd.Run(), stderr.String())
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join(out, name))
		require.NoError(t, err, name)
		return string(data)
	}
	return helperOutput{read(requirementsName), read(packagesName), read(lockConstraintsName), strings.ReplaceAll(stderr.String(), "\r\n", "\n")}
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

// helperRequirementCases are pyproject.toml files and the requirements.txt the
// install helper writes for each, with no lock. TestInstallHelperRequirements
// holds the helper to them, and TestInstallHelperAgreesWithTheManifest holds
// pkg/manifest to the same reading.
var helperRequirementCases = map[string]struct {
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
	"the Airflow sources astro init writes add no page": {
		pyproject: "[project]\ndependencies = ['apache-airflow==3.3.*', 'apache-airflow-core', 'apache-airflow-task-sdk', 'Private_Lib>=1']\n" +
			"[[tool.uv.index]]\nname = 'astronomer'\nurl = 'https://pip.astronomer.io/v2/'\nexplicit = true\n" +
			"[[tool.uv.index]]\nname = 'acme'\nurl = 'https://pypi.acme.example/simple'\n" +
			"[tool.uv.sources]\napache-airflow = { index = 'astronomer' }\napache-airflow-core = { index = 'astronomer' }\n" +
			"apache-airflow-task-sdk = { index = 'astronomer' }\nPrivate_Lib = { index = 'acme' }\n",
		want: "--find-links https://pypi.acme.example/simple/Private_Lib/\nPrivate_Lib>=1\n",
	},
	"credentials are left out of every URL": {
		pyproject: "[project]\ndependencies = ['private-lib', 'tool']\n" +
			"[[tool.uv.index]]\nname = 'acme'\nurl = 'https://user:s3cret@pypi.acme.example/simple/'\n" +
			"[tool.uv.sources]\nprivate-lib = { index = 'acme' }\ntool = { url = 'https://token@files.acme.example/tool-1.0.tar.gz' }\n",
		want: "--find-links https://pypi.acme.example/simple/private-lib/\nprivate-lib\ntool @ https://files.acme.example/tool-1.0.tar.gz\n",
	},
}

// The helper turns [project] dependencies and [tool.uv.sources] into the
// requirements the runtime's install reads, as the manifest reads them: no
// Airflow, a git or URL source as a direct reference, an index source as a
// --find-links page ahead of everything.
func TestInstallHelperRequirements(t *testing.T) {
	for name, tc := range helperRequirementCases {
		t.Run(name, func(t *testing.T) {
			reqs, _, constraints := installLists(t, helperProject(t, tc.pyproject, ""))
			assert.Equal(t, tc.want, reqs)
			assert.Empty(t, constraints, "no lock, no constraints")
		})
	}
}

// The helper and pkg/manifest read a pyproject.toml the same way, so a check
// environment (manifest.Requirements, UV.IndexPagesFor) and an image can't
// drift: for each case, the helper's requirements.txt is the manifest's
// requirements without Airflow after its index pages, with credentials left
// out of URLs, the one difference the helper makes on purpose.
func TestInstallHelperAgreesWithTheManifest(t *testing.T) {
	for name, tc := range helperRequirementCases {
		t.Run(name, func(t *testing.T) {
			// A manifest pkg/manifest loads: a name, an Airflow requirement and
			// [tool.astro], none of which changes what is installed.
			pyproject := tc.pyproject
			if !strings.Contains(pyproject, "[project]\nname = ") {
				pyproject = strings.Replace(pyproject, "[project]\n", "[project]\nname = 'p'\n", 1)
			}
			if !strings.Contains(pyproject, "'apache-airflow==") {
				if strings.Contains(pyproject, "dependencies = [") {
					pyproject = strings.Replace(pyproject, "dependencies = [", "dependencies = ['apache-airflow==3.3.*', ", 1)
				} else {
					pyproject = strings.Replace(pyproject, "[project]\n", "[project]\ndependencies = ['apache-airflow==3.3.*']\n", 1)
				}
			}
			pyproject += "\n[tool.astro]\n"
			m, err := manifest.ParseForRepair([]byte(pyproject))
			require.NoError(t, err)
			deps := runtimeDeps(m.Requirements())
			lines := make([]string, 0, len(deps)+1)
			for _, page := range m.UV.IndexPagesFor(deps) {
				lines = append(lines, "--find-links "+page)
			}
			lines = append(lines, deps...)
			want := withoutCredentials(strings.Join(lines, "\n") + "\n")

			reqs, _, _ := installLists(t, helperProject(t, pyproject, ""))
			assert.Equal(t, want, reqs)
		})
	}
}

// credentialsRe is the user and password an http(s) URL can carry.
var credentialsRe = regexp.MustCompile(`(https?://)[^/@\s]+@`)

// withoutCredentials is s with credentialsRe taken out, as the helper writes
// URLs.
func withoutCredentials(s string) string {
	return credentialsRe.ReplaceAllString(s, "$1")
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
//
// A dependency may end in "|" and the source fields uv records for it
// (`|index = "https://…"`, `|git = "https://…?rev=v1"`), written into the
// lock's record and not into pyproject.toml; uv records no specifier for a
// git, URL or path source.
func lockProject(deps []string, packages string) (pyproject, lock string) {
	quoted := make([]string, len(deps))
	requiresDist := make([]string, len(deps))
	for i, entry := range deps {
		d, source, _ := strings.Cut(entry, "|")
		quoted[i] = "'" + d + "'"
		name, spec := d, ""
		if at := strings.IndexAny(d, "<>=!~"); at >= 0 {
			name, spec = d[:at], d[at:]
		}
		requiresDist[i] = `{ name = "` + name + `"`
		if spec != "" && !strings.HasPrefix(source, "git") && !strings.HasPrefix(source, "url") && !strings.HasPrefix(source, "directory") {
			requiresDist[i] += `, specifier = "` + spec + `"`
		}
		if source != "" {
			requiresDist[i] += ", " + source
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
	pyproject, lock := lockProject([]string{`apache-airflow==3.3.*|index = "https://pip.astronomer.io/v2/"`, `private-lib|index = "https://pypi.acme.example/simple/"`, "pandas"}, lockEntry("pandas", "2.2.3")+`
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
	lines := strings.Split(strings.ReplaceAll(strings.TrimSpace(string(got)), "\r\n", "\n"), "\n")
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
	require.NoFileExists(t, filepath.Join(dir, installFindLinksName))
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
	assert.NotContains(t, inputs, installFindLinksName)
}

// A dependency with a version whose source is a git repository, a URL or a
// path is recorded by uv with that source and no specifier. A fresh lock of it
// is not stale; one recorded with another source than pyproject.toml now
// names, or with none, is.
func TestInstallHelperStaleCheckFollowsSources(t *testing.T) {
	const gitLock = `[[package]]
name = "private-lib"
version = "1.2.0"
source = { git = "https://github.com/acme/private-lib?rev=v1.2.0#0123456789abcdef0123456789abcdef01234567" }
`
	const pyprojectGit = "\n[tool.uv.sources]\nprivate-lib = { git = 'https://github.com/acme/private-lib', rev = 'v1.2.0' }\n"
	const pyprojectNewIndex = "\n[[tool.uv.index]]\nname = 'acme'\nurl = 'https://new.acme.example/simple/'\nexplicit = true\n" +
		"[tool.uv.sources]\nprivate-lib = { index = 'acme' }\n"
	for name, tc := range map[string]struct {
		source, sources string
		stale           bool
	}{
		"a fresh lock of a git source":       {`git = "https://github.com/acme/private-lib?rev=v1.2.0"`, pyprojectGit, false},
		"a fresh lock of a URL source":       {`url = "https://files.acme.example/private_lib-1.2.0-py3-none-any.whl"`, "\n[tool.uv.sources]\nprivate-lib = { url = 'https://files.acme.example/private_lib-1.2.0-py3-none-any.whl' }\n", false},
		"a fresh lock of a path source":      {`directory = "libs/private-lib"`, "\n[tool.uv.sources]\nprivate-lib = { path = 'libs/private-lib' }\n", false},
		"the git ref moved":                  {`git = "https://github.com/acme/private-lib?rev=v1.1.0"`, pyprojectGit, true},
		"a source added since the lock":      {"", pyprojectGit, true},
		"a source taken away since the lock": {`git = "https://github.com/acme/private-lib?rev=v1.2.0"`, "", true},
		"the index moved":                    {`index = "https://old.acme.example/simple/"`, pyprojectNewIndex, true},
	} {
		t.Run(name, func(t *testing.T) {
			dep := "private-lib>=1.2"
			if tc.source != "" {
				dep += "|" + tc.source
			}
			pyproject, lock := lockProject([]string{dep, "pandas"}, lockEntry("pandas", "2.2.3")+gitLock)
			out := runHelper(t, helperProject(t, pyproject+tc.sources, lock), helperRun{})
			if tc.stale {
				assert.Contains(t, out.stderr, "uv.lock is out of date")
				assert.Empty(t, out.constraints)
				return
			}
			assert.Empty(t, out.stderr)
			assert.Contains(t, out.constraints, "pandas==2.2.3")
		})
	}
}

// The install reads PyPI, so a package the lock took from another index (a
// mirror, a private index the project reads by default) is not pinned: its
// version may not be on PyPI, and the install would find no distribution.
func TestInstallHelperPinsOnlyWhatPyPIServes(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas", "internal-tool"}, lockEntry("pandas", "2.2.3")+`[[package]]
name = "internal-tool"
version = "4.0.1"
source = { registry = "https://artifactory.acme.example/api/pypi/simple" }

[[package]]
name = "numpy"
version = "2.4.6"
source = { registry = "https://pypi.org/simple/" }
`)
	out := runHelper(t, helperProject(t, pyproject, lock), helperRun{})
	assert.Equal(t, "# Generated from uv.lock by .astro/install-project.py.\n"+
		"# Not pinned, uv.lock took them from an index other than PyPI: internal-tool\n"+
		"numpy==2.4.6\npandas==2.2.3\n", out.constraints)
	assert.Contains(t, out.stderr, "uv.lock took internal-tool from an index other than PyPI")
}

// What the helper writes lands in an image layer and the build log, so a user
// or token in an index or source URL is left out of it, with a warning: the
// build authenticates through the netrc secret.
func TestInstallHelperLeavesCredentialsOut(t *testing.T) {
	const pyproject = "[project]\nname = 'p'\ndependencies = ['private-lib', 'tool', 'pandas']\n" +
		"[[tool.uv.index]]\nname = 'acme'\nurl = 'https://user:s3cret@pypi.acme.example/simple/'\nexplicit = true\n" +
		"[tool.uv.sources]\nprivate-lib = { index = 'acme' }\n" +
		"tool = { git = 'https://x-access-token:ghp_s3cret@github.com/acme/tool', rev = 'v1' }\n"
	out := runHelper(t, helperProject(t, pyproject, ""), helperRun{})
	assert.Equal(t, "--find-links https://pypi.acme.example/simple/private-lib/\nprivate-lib\n"+
		"tool @ git+https://github.com/acme/tool@v1\npandas\n", out.requirements)
	assert.NotContains(t, out.requirements, "s3cret")
	assert.Equal(t, 1, strings.Count(out.stderr, "carries a user or password"), out.stderr)

	// An ssh URL's user is how ssh logs in, not a credential.
	out = runHelper(t, helperProject(t, "[project]\nname = 'p'\ndependencies = ['tool']\n[tool.uv.sources]\ntool = { git = 'ssh://git@github.com/acme/tool' }\n", ""), helperRun{})
	assert.Equal(t, "tool @ git+ssh://git@github.com/acme/tool\n", out.requirements)
	assert.Empty(t, out.stderr)

	// Nor do the lock's file URLs carry one.
	pyproject2, lock := lockProject([]string{`private-lib|index = "https://pypi.acme.example/simple/"`}, `[[package]]
name = "private-lib"
version = "1.2.0"
source = { registry = "https://pypi.acme.example/simple/" }
sdist = { url = "https://user:s3cret@pypi.acme.example/files/private_lib-1.2.0.tar.gz", hash = "sha256:aaaa" }
`)
	pyproject2 += "\n[[tool.uv.index]]\nname = 'acme'\nurl = 'https://pypi.acme.example/simple/'\nexplicit = true\n[tool.uv.sources]\nprivate-lib = { index = 'acme' }\n"
	out = runHelper(t, helperProject(t, pyproject2, lock), helperRun{})
	assert.Contains(t, out.requirements, "private-lib @ https://pypi.acme.example/files/private_lib-1.2.0.tar.gz#sha256=aaaa\n")
	assert.NotContains(t, out.requirements, "s3cret")
}

// What the base image has installed stays at the runtime's versions, its
// tested ones, unless the project names the package itself: in its
// dependencies, its optional dependencies or a dependency group. On every
// runtime, whether or not it ships a constraints file.
func TestInstallHelperLeavesTheBaseImagesPackages(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas"}, lockEntry("pandas", "2.2.3")+lockEntry("six", "1.17.0")+
		lockEntry("python-dateutil", "2.9.0")+lockEntry("pytz", "2026.5")+lockEntry("tzdata", "2026.5")+lockEntry("numpy", "2.4.6"))
	pyproject += "[project.optional-dependencies]\ntime = ['python-dateutil']\n" +
		"\n[dependency-groups]\ndev = ['pytz', { include-group = 'other' }]\nother = []\n"
	dir := helperProject(t, pyproject, lock)
	base := []string{"ASTRO_INSTALL_PROJECT_BASE_INSTALLED=pandas,Six,python-dateutil,pytz,tzdata,apache-airflow"}
	const want = "# Generated from uv.lock by .astro/install-project.py.\n" +
		"numpy==2.4.6\npandas==2.2.3\npython-dateutil==2.9.0\npytz==2026.5\n"

	for name, constraints := range map[string]string{"a runtime with a constraints file": "", "one without": filepath.Join(t.TempDir(), "absent")} {
		t.Run(name, func(t *testing.T) {
			out := runHelper(t, dir, helperRun{runtimeConstraints: constraints, env: base})
			assert.Equal(t, want, out.constraints,
				"six and tzdata are the base's and not the project's; pandas, python-dateutil and pytz are the project's own; numpy is not in the base")
		})
	}
}

// A project that keeps its files read-only ships them read-only into the
// context, and the build still writes its own install inputs over them.
func TestBuildReplacesReadOnlyProjectFiles(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"pyproject.toml":  "[project]\nname = 'p'\ndependencies = ['pandas']\n",
		"uv.lock":         "version = 1\n",
		InstallHelperPath: "# an older helper\n",
	})
	for _, name := range []string{"pyproject.toml", "uv.lock", InstallHelperPath} {
		require.NoError(t, os.Chmod(filepath.Join(project, filepath.FromSlash(name)), 0o444))
	}
	req := testRequest(t)
	req.ManifestDir, req.ProjectFiles = project, project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	helper, err := os.ReadFile(filepath.Join(req.WorkDir, buildContextDir, filepath.FromSlash(generatedInputsDir), filepath.Base(InstallHelperPath)))
	require.NoError(t, err)
	assert.Equal(t, InstallHelper(), helper, "the CLI's helper, not the project's")
}

// With a project's pyproject.toml to install from, that file decides whether
// there is anything to install, not the request's own lists.
func TestBuildFastPathFollowsTheProjectsManifest(t *testing.T) {
	for name, tc := range map[string]struct {
		pyproject    string
		dependencies []string
		builds       bool
	}{
		"the project lists a dependency the request does not": {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*', 'pandas']\n[tool.astro]\n", nil, true},
		"the project lists an OS package":                     {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n[tool.astro]\npackages = ['libpq-dev']\n", nil, true},
		"the project lists only Airflow":                      {"[project]\nname = 'p'\ndependencies = ['apache-airflow==3.1.*']\n[tool.astro]\n", []string{"pandas"}, false},
		"a pyproject.toml that does not parse":                {"[project\n", nil, true},
	} {
		t.Run(name, func(t *testing.T) {
			project := t.TempDir()
			writeTree(t, project, map[string]string{"pyproject.toml": tc.pyproject})
			cmd := &fakeCmd{}
			req := testRequest(t)
			req.ManifestDir, req.Dependencies = project, tc.dependencies
			image, err := testBuilder(cmd).Build(t.Context(), req, rt.Callbacks{})
			require.NoError(t, err)
			if tc.builds {
				assert.Equal(t, req.Tag, image)
				assert.NotEmpty(t, cmd.calls)
				return
			}
			assert.Equal(t, req.BaseImage+"-base", image)
			assert.Empty(t, cmd.calls)
		})
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// A lock with no record of the project cannot say what it resolved the
// project's dependencies for, so it is stale, not fresh.
func TestInstallHelperTreatsALockWithoutTheProjectAsStale(t *testing.T) {
	out := runHelper(t, helperProject(t, "[project]\nname = 'p'\ndependencies = ['pandas']\n", "version = 1\n\n"+lockEntry("pandas", "2.2.3")), helperRun{})
	assert.Contains(t, out.stderr, "uv.lock is out of date with pyproject.toml (it has no record of the project)")
	assert.Empty(t, out.constraints)
}

// uv records [tool.uv] constraint-dependencies and override-dependencies in the
// lock's [manifest]; a lock resolved under other ones is stale. The same ones
// written another way are not.
func TestInstallHelperStaleCheckReadsConstraintsAndOverrides(t *testing.T) {
	pyproject, lock := lockProject([]string{"pandas"}, lockEntry("pandas", "2.2.3"))
	lock = strings.Replace(lock, "version = 1\n", "version = 1\n\n[manifest]\n"+
		`constraints = [{ name = "numpy", specifier = "<3,>=2" }, { name = "sqlalchemy", specifier = "==2.0.50", marker = "python_version >= '3.12'" }]`+"\n"+
		`overrides = [{ name = "protobuf", specifier = "==6.33.6" }]`+"\n", 1)
	const uv = "\n[tool.uv]\nconstraint-dependencies = ['SQLAlchemy==2.0.50; python_version >= \"3.12\"', 'numpy>=2,<3']\n" +
		"override-dependencies = ['protobuf==6.33.6']\n"
	for name, tc := range map[string]struct {
		uv    string
		stale string
	}{
		"the same, written another way":    {uv, ""},
		"a constraint changed":             {strings.Replace(uv, "numpy>=2,<3", "numpy>=2,<2.5", 1), "constraint-dependencies"},
		"a constraint added":               {strings.Replace(uv, "'numpy>=2,<3'", "'numpy>=2,<3', 'pytz'", 1), "constraint-dependencies"},
		"an override taken away":           {strings.Replace(uv, "override-dependencies = ['protobuf==6.33.6']\n", "", 1), "override-dependencies"},
		"none at all, though uv used some": {"", "constraint-dependencies"},
	} {
		t.Run(name, func(t *testing.T) {
			out := runHelper(t, helperProject(t, pyproject+tc.uv, lock), helperRun{})
			if tc.stale == "" {
				assert.Empty(t, out.stderr)
				assert.Contains(t, out.constraints, "pandas==2.2.3")
				return
			}
			assert.Contains(t, out.stderr, "it was resolved under other [tool.uv] "+tc.stale)
			assert.Empty(t, out.constraints)
		})
	}
}

// Only the CLI's build of a request naming no project writes a find-links
// file. A project that carries one (left by an older build) has it taken out
// of the context, and it is never read.
func TestBuildDropsAFindLinksFileTheProjectCarries(t *testing.T) {
	project := t.TempDir()
	writeTree(t, project, map[string]string{
		"pyproject.toml":                 "[project]\nname = 'p'\ndependencies = ['pandas']\n[tool.astro]\n",
		".astro/" + installFindLinksName: "https://evil.example/simple/pandas/\n",
	})
	req := testRequest(t)
	req.ManifestDir, req.ProjectFiles = project, project
	req.Dependencies = []string{"pandas"}

	_, err := testBuilder(&fakeCmd{}).Build(t.Context(), req, rt.Callbacks{})
	require.NoError(t, err)
	ctxDir := filepath.Join(req.WorkDir, buildContextDir)
	assert.NoFileExists(t, filepath.Join(ctxDir, ".astro", installFindLinksName))
	assert.NoFileExists(t, filepath.Join(ctxDir, filepath.FromSlash(generatedInputsDir), installFindLinksName))
	reqs, _, _ := installLists(t, ctxDir)
	assert.Equal(t, "pandas\n", reqs)
}
